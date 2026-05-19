package service

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"server/config"
	"server/global"
	"server/model/elasticsearch"
	"server/model/request"
	"server/model/response"
	"server/utils"

	"github.com/elastic/go-elasticsearch/v8/typedapi/core/search"
	"github.com/elastic/go-elasticsearch/v8/typedapi/types"
	"go.uber.org/zap"
)

type AIService struct {
}

func (aiService *AIService) ListModels() response.AIModelList {
	models := aiService.buildModelResponses(global.Config.AI.EffectiveChatModels())
	return response.AIModelList{
		CurrentModel:   global.Config.AI.CurrentChatModel(),
		EmbeddingModel: global.Config.AI.QwenEmbeddingModel,
		Models:         models,
		Total:          len(models),
	}
}

func (aiService *AIService) CreateModel(req request.AIModel) error {
	model := requestToConfigAIModel(req)
	if strings.TrimSpace(model.Name) == "" {
		return errors.New("model name is required")
	}
	models := global.Config.AI.EffectiveChatModels()
	if findAIModelIndex(models, model.Name, model.ExpireAt) >= 0 {
		return errors.New("model already exists")
	}
	models = append(models, model)
	global.Config.AI.ChatModels = models
	return utils.SaveYAML()
}

func (aiService *AIService) BulkCreateModels(req request.AIBulkModels) (response.AIBulkCreate, error) {
	models := global.Config.AI.EffectiveChatModels()
	seen := make(map[string]struct{}, len(models))
	for _, model := range models {
		seen[aiModelKey(model.Name, model.ExpireAt)] = struct{}{}
	}

	result := response.AIBulkCreate{Skipped: make([]string, 0)}
	for _, item := range req.Models {
		model := requestToConfigAIModel(item)
		if strings.TrimSpace(model.Name) == "" {
			continue
		}
		key := aiModelKey(model.Name, model.ExpireAt)
		if _, ok := seen[key]; ok {
			result.Skipped = append(result.Skipped, model.Name)
			continue
		}
		seen[key] = struct{}{}
		models = append(models, model)
		result.Created++
	}
	if result.Created == 0 {
		result.Models = aiService.buildModelResponses(models)
		return result, nil
	}
	global.Config.AI.ChatModels = models
	if err := utils.SaveYAML(); err != nil {
		return response.AIBulkCreate{}, err
	}
	result.Models = aiService.buildModelResponses(global.Config.AI.EffectiveChatModels())
	return result, nil
}

func (aiService *AIService) UpdateModel(req request.AIModelUpdate) error {
	models := global.Config.AI.EffectiveChatModels()
	index := findAIModelIndex(models, req.OldName, req.OldExpireAt)
	if index < 0 {
		return errors.New("model not found")
	}
	next := config.AIModel{
		Name:       strings.TrimSpace(req.Name),
		ExpireAt:   strings.TrimSpace(req.ExpireAt),
		ShowInChat: req.ShowInChat,
	}
	if next.Name == "" {
		return errors.New("model name is required")
	}
	for i, model := range models {
		if i == index {
			continue
		}
		if aiModelKey(model.Name, model.ExpireAt) == aiModelKey(next.Name, next.ExpireAt) {
			return errors.New("model already exists")
		}
	}
	oldName := models[index].Name
	models[index] = next
	if global.Config.AI.QwenModel == oldName {
		global.Config.AI.QwenModel = next.Name
	}
	global.Config.AI.ChatModels = models
	return utils.SaveYAML()
}

func (aiService *AIService) DeleteModel(req request.AIModelDelete) error {
	models := global.Config.AI.EffectiveChatModels()
	index := findAIModelIndex(models, req.Name, req.ExpireAt)
	if index < 0 {
		return errors.New("model not found")
	}
	deletedName := models[index].Name
	models = append(models[:index], models[index+1:]...)
	if global.Config.AI.QwenModel == deletedName && !hasAIModelName(models, deletedName) {
		if len(models) > 0 {
			global.Config.AI.QwenModel = models[0].Name
		} else {
			global.Config.AI.QwenModel = ""
		}
	}
	global.Config.AI.ChatModels = models
	return utils.SaveYAML()
}

func (aiService *AIService) SetCurrentModel(model string) error {
	model = strings.TrimSpace(model)
	if model == "" {
		return errors.New("model is required")
	}
	if !hasAIModelName(global.Config.AI.EffectiveChatModels(), model) {
		return errors.New("model not found")
	}
	global.Config.AI.QwenModel = model
	return utils.SaveYAML()
}

// aiContextArticle 是第一版关键词检索遗留的文章级上下文结构。
// 当前第二版主流程已经切到 RagChunk，保留这些函数是为了后续需要回退关键词检索时复用。
type aiContextArticle struct {
	ID      string
	Source  elasticsearch.Article
	Snippet string
}

// qwenChatRequest/qwenChatResponse 对应千问 compatible-mode 的 /chat/completions 接口。
// 这里没有引入 SDK，是为了和当前项目风格保持一致，直接用标准库 http 调用。
type qwenChatRequest struct {
	Model       string            `json:"model"`
	Messages    []qwenChatMessage `json:"messages"`
	Temperature float64           `json:"temperature,omitempty"`
	MaxTokens   int               `json:"max_tokens,omitempty"`
	Stream      bool              `json:"stream"`
}

type qwenChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type qwenChatResponse struct {
	Choices []struct {
		Message      qwenChatMessage `json:"message"`
		FinishReason string          `json:"finish_reason"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
		Code    string `json:"code"`
	} `json:"error,omitempty"`
}

type qwenChatStreamResponse struct {
	Choices []struct {
		Delta        qwenChatMessage `json:"delta"`
		Message      qwenChatMessage `json:"message"`
		FinishReason string          `json:"finish_reason"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
		Code    string `json:"code"`
	} `json:"error,omitempty"`
}

// Chat 是 AI 聊天的总入口。
// 第二版流程：用户问题 -> RAG 混合检索拿到 chunks -> 组装 Prompt -> 调千问生成最终回答。
func (aiService *AIService) Chat(req request.AIChat) (response.AIChat, error) {
	question := strings.TrimSpace(req.Message)
	if question == "" {
		return response.AIChat{}, errors.New("message is required")
	}
	if !global.Config.AI.Enable {
		return response.AIChat{}, errors.New("AI chat is disabled")
	}
	topK := global.Config.AI.TopK
	if topK <= 0 {
		topK = 8
	}
	hits, err := aiService.retrieveHits(context.TODO(), question, topK)
	if err != nil {
		return response.AIChat{}, err
	}
	sources := aiService.buildSources(hits)
	answer, err := aiService.callQwen(context.TODO(), req, hits)
	if err != nil {
		return response.AIChat{}, err
	}
	return response.AIChat{
		Answer:  strings.TrimSpace(answer),
		Sources: sources,
	}, nil
}

// ChatStream 和 Chat 使用同一套检索/Prompt，只是把大模型输出以增量形式回调给 API 层。
func (aiService *AIService) ChatStream(ctx context.Context, req request.AIChat, onSources func([]response.AIArticle) error, onDelta func(string) error) error {
	question := strings.TrimSpace(req.Message)
	if question == "" {
		return errors.New("message is required")
	}
	if !global.Config.AI.Enable {
		return errors.New("AI chat is disabled")
	}
	topK := global.Config.AI.TopK
	if topK <= 0 {
		topK = 8
	}
	hits, err := aiService.retrieveHits(ctx, question, topK)
	if err != nil {
		return err
	}
	if onSources != nil {
		if err := onSources(aiService.buildSources(hits)); err != nil {
			return err
		}
	}
	return aiService.callQwenStream(ctx, req, hits, onDelta)
}

func (aiService *AIService) buildModelResponses(models []config.AIModel) []response.AIChatModel {
	currentModel := global.Config.AI.CurrentChatModel()
	result := make([]response.AIChatModel, 0, len(models))
	for _, model := range models {
		result = append(result, response.AIChatModel{
			Name:       model.Name,
			ExpireAt:   model.ExpireAt,
			Current:    model.Name == currentModel,
			ShowInChat: model.ShowInChat,
		})
	}
	return result
}

func requestToConfigAIModel(req request.AIModel) config.AIModel {
	return config.AIModel{
		Name:       strings.TrimSpace(req.Name),
		ExpireAt:   strings.TrimSpace(req.ExpireAt),
		ShowInChat: req.ShowInChat,
	}
}

func findAIModelIndex(models []config.AIModel, name string, expireAt string) int {
	key := aiModelKey(name, expireAt)
	for index, model := range models {
		if aiModelKey(model.Name, model.ExpireAt) == key {
			return index
		}
	}
	return -1
}

func hasAIModelName(models []config.AIModel, name string) bool {
	name = strings.TrimSpace(name)
	for _, model := range models {
		if model.Name == name {
			return true
		}
	}
	return false
}

func aiModelKey(name string, expireAt string) string {
	return strings.TrimSpace(name) + "\x00" + strings.TrimSpace(expireAt)
}

func (aiService *AIService) retrieveHits(ctx context.Context, question string, topK int) ([]RAGSearchHit, error) {
	// 向量库只存中文博客内容，所以检索 filter 固定为 zh。
	// 用户如果用英文提问，Embedding 仍然可以检索中文知识，最终回答语言由 Prompt 控制。
	hits, err := ServiceGroupApp.RAGService.HybridSearch(ctx, question, "zh", topK)
	if err == nil && len(hits) > 0 {
		return hits, nil
	}
	if err != nil {
		global.Log.Warn("Hybrid RAG search failed, fallback to keyword chunks", zap.Error(err))
	}

	hits, keywordErr := ServiceGroupApp.RAGService.KeywordSearchChunks(ctx, question, "zh", topK)
	if keywordErr == nil && len(hits) > 0 {
		return hits, nil
	}
	if keywordErr != nil {
		global.Log.Warn("RAG keyword chunk search failed, fallback to article search", zap.Error(keywordErr))
	}

	articleHits, articleErr := aiService.articleFallbackHits(ctx, question, topK)
	if articleErr != nil {
		if err != nil {
			return nil, err
		}
		if keywordErr != nil {
			return nil, keywordErr
		}
		return nil, articleErr
	}
	return articleHits, nil
}

func (aiService *AIService) articleFallbackHits(ctx context.Context, question string, topK int) ([]RAGSearchHit, error) {
	articles, err := aiService.searchArticles(ctx, question)
	if err != nil {
		return nil, err
	}
	if topK <= 0 || topK > len(articles) {
		topK = len(articles)
	}
	hits := make([]RAGSearchHit, 0, topK)
	for i := 0; i < topK; i++ {
		article := articles[i]
		hits = append(hits, RAGSearchHit{
			ID:        "article-" + article.ID,
			ArticleID: article.ID,
			Text:      article.Snippet,
			Title:     article.Source.Title,
			URL:       ServiceGroupApp.RAGService.ArticleURL(article.ID),
			Language:  "zh",
		})
	}
	return hits, nil
}

// searchArticles 是第一版文章级关键词检索逻辑。
// 第二版不再走这里；保留它可以作为后续 Hybrid Search 失败时的兜底方案。
func (aiService *AIService) searchArticles(ctx context.Context, query string) ([]aiContextArticle, error) {
	topK := global.Config.AI.TopK
	if topK <= 0 {
		topK = 8
	}
	req := &search.Request{
		Query: &types.Query{
			Bool: &types.BoolQuery{
				Should: []types.Query{
					{Match: map[string]types.MatchQuery{"title": {Query: query}}},
					{Match: map[string]types.MatchQuery{"keyword": {Query: query}}},
					{Match: map[string]types.MatchQuery{"abstract": {Query: query}}},
					{Match: map[string]types.MatchQuery{"content": {Query: query}}},
				},
				MinimumShouldMatch: 1,
			},
		},
		Size: &topK,
	}
	res, err := global.ESClient.Search().
		Index(elasticsearch.ArticleIndex()).
		Request(req).
		SourceIncludes_("title", "abstract", "content", "category", "tags", "created_at").
		Do(ctx)
	if err != nil {
		return nil, err
	}

	articles := make([]aiContextArticle, 0, len(res.Hits.Hits))
	for _, hit := range res.Hits.Hits {
		var article elasticsearch.Article
		if err := json.Unmarshal(hit.Source_, &article); err != nil {
			continue
		}
		id := ""
		if hit.Id_ != nil {
			id = *hit.Id_
		}
		articles = append(articles, aiContextArticle{
			ID:      id,
			Source:  article,
			Snippet: aiService.articleSnippet(article, query),
		})
	}
	return articles, nil
}

// callQwen 把检索到的 chunks 和用户问题一起发送给千问大模型。
// 这里调用的是 Chat 模型，不是 Embedding 模型；Embedding 模型只在 RAGService 中使用。
func (aiService *AIService) callQwen(ctx context.Context, req request.AIChat, hits []RAGSearchHit) (string, error) {
	apiKey := strings.TrimSpace(global.Config.AI.QwenAPIKey)
	if apiKey == "" {
		return "", errors.New("qwen api key is not configured")
	}
	baseURL := strings.TrimRight(global.Config.AI.QwenBaseURL, "/")
	if baseURL == "" {
		baseURL = "https://dashscope.aliyuncs.com/compatible-mode/v1"
	}
	model, err := aiService.resolveChatModel(req.Model)
	if err != nil {
		return "", err
	}
	timeout := global.Config.AI.RequestTimeout
	if timeout <= 0 {
		timeout = 60
	}
	ctx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
	defer cancel()

	payload := qwenChatRequest{
		Model:       model,
		Messages:    aiService.buildMessages(req, hits),
		Temperature: global.Config.AI.Temperature,
		Stream:      false,
	}
	// max_tokens 太低时，模型会像“话没说完”一样被截断。这里给聊天回答一个可读性下限。
	payload.MaxTokens = global.Config.AI.MaxTokens
	if payload.MaxTokens <= 0 || payload.MaxTokens < 2200 {
		payload.MaxTokens = 2200
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	var qwenResp qwenChatResponse
	if err := json.Unmarshal(respBody, &qwenResp); err != nil {
		return "", err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if qwenResp.Error != nil && qwenResp.Error.Message != "" {
			return "", errors.New(qwenResp.Error.Message)
		}
		return "", fmt.Errorf("qwen request failed: %s", resp.Status)
	}
	if len(qwenResp.Choices) == 0 {
		return "", errors.New("qwen response has no choices")
	}
	return aiService.normalizeChatAnswer(qwenResp.Choices[0].Message.Content), nil
}

func (aiService *AIService) callQwenStream(ctx context.Context, req request.AIChat, hits []RAGSearchHit, onDelta func(string) error) error {
	apiKey := strings.TrimSpace(global.Config.AI.QwenAPIKey)
	if apiKey == "" {
		return errors.New("qwen api key is not configured")
	}
	baseURL := strings.TrimRight(global.Config.AI.QwenBaseURL, "/")
	if baseURL == "" {
		baseURL = "https://dashscope.aliyuncs.com/compatible-mode/v1"
	}
	model, err := aiService.resolveChatModel(req.Model)
	if err != nil {
		return err
	}
	timeout := global.Config.AI.RequestTimeout
	if timeout <= 0 {
		timeout = 60
	}
	ctx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
	defer cancel()

	payload := qwenChatRequest{
		Model:       model,
		Messages:    aiService.buildMessages(req, hits),
		Temperature: global.Config.AI.Temperature,
		Stream:      true,
		MaxTokens:   global.Config.AI.MaxTokens,
	}
	if payload.MaxTokens <= 0 || payload.MaxTokens < 2200 {
		payload.MaxTokens = 2200
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		var qwenResp qwenChatResponse
		if err := json.Unmarshal(respBody, &qwenResp); err == nil && qwenResp.Error != nil && qwenResp.Error.Message != "" {
			return errors.New(qwenResp.Error.Message)
		}
		return fmt.Errorf("qwen stream request failed: %s", resp.Status)
	}

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "[DONE]" {
			return nil
		}
		var streamResp qwenChatStreamResponse
		if err := json.Unmarshal([]byte(data), &streamResp); err != nil {
			continue
		}
		if streamResp.Error != nil && streamResp.Error.Message != "" {
			return errors.New(streamResp.Error.Message)
		}
		for _, choice := range streamResp.Choices {
			delta := choice.Delta.Content
			if delta == "" {
				delta = choice.Message.Content
			}
			if delta != "" && onDelta != nil {
				if err := onDelta(delta); err != nil {
					return err
				}
			}
		}
	}
	return scanner.Err()
}

func (aiService *AIService) resolveChatModel(requestedModel string) (string, error) {
	model := strings.TrimSpace(requestedModel)
	if model == "" {
		return global.Config.AI.CurrentChatModel(), nil
	}
	for _, item := range global.Config.AI.VisibleChatModels() {
		if item.Name == model {
			return model, nil
		}
	}
	return "", errors.New("selected chat model is not available")
}

// buildMessages 负责生成 OpenAI-compatible 的 messages 数组。
// 优化理念：System message 专门负责设定角色、工作流和严格格式，不包含具体数据。
func (aiService *AIService) buildMessages(req request.AIChat, hits []RAGSearchHit) []qwenChatMessage {
	languageRule := "请使用与用户最后一个问题相同的语言回答；如果用户使用英文提问，请使用英文回答。"
	if strings.EqualFold(req.Language, "en") || looksEnglish(req.Message) {
		languageRule = "Answer in English because the user is asking in English. The retrieved blog snippets may be Chinese; translate and explain them naturally in English."
	}

	systemPrompt := strings.Join([]string{
		"【角色与目标】",
		"你是天奇个人博客的专属 AI 助手同时。你的目标是自然、热情且专业地解答用户的疑问。",
		"",
		"【核心工作流（必须严格遵循）】",
		"1. 优先基于检索到的博客片段作答。提取相关信息，自然地融入对话，切勿将不相关的片段生硬拼接成答案。",
		"2. 若博客片段与问题无关，或信息不足以回答问题，你必须先明确声明：“在天奇的博客中暂时没有找到相关内容。”",
		"3. 声明之后，必须无缝切换到通用知识模式。只要用户询问的是常识、技术名词、概念或历史文化等问题，都要继续给出详尽、完整的解释，并说明这部分是为你补充的拓展知识。",
		"",
		"【回答原则】",
		"1. 拒绝半途而废：禁止只回答“没有找到相关信息”就结束对话，必须提供有价值的补充。",
		"2. 深度与连贯：回答要像高质量的专栏探讨一样清楚、完整。遇到复杂问题需深入剖析，遇到简单问题则精准解答，切忌说到一半戛然而止。",
		"",
		"【格式要求】",
		"1. 可以使用简洁 Markdown 来提升可读性，例如小标题、列表、行内代码和代码块。",
		"2. 涉及代码时必须使用 Markdown 代码块包裹，避免代码和正文混在一起。",
		"3. 链接处理：前端会自动展示参考文章的卡片或链接，因此你的正文回答中严禁再次粘贴任何文章链接。",
		languageRule,
	}, "\n")

	messages := []qwenChatMessage{{Role: "system", Content: systemPrompt}}

	history := req.History
	// 只保留最近 6 条历史，避免历史对话过长挤占博客上下文 token，导致模型遗忘指令。
	if len(history) > 6 {
		history = history[len(history)-6:]
	}

	for _, item := range history {
		role := "user"
		if item.Role == "assistant" || item.Role == "bot" {
			role = "assistant"
		}
		content := strings.TrimSpace(item.Content)
		if content == "" {
			continue
		}
		messages = append(messages, qwenChatMessage{Role: role, Content: truncateRunes(content, 1000)})
	}

	userPrompt := aiService.buildUserPrompt(strings.TrimSpace(req.Message), hits)
	messages = append(messages, qwenChatMessage{Role: "user", Content: userPrompt})

	return messages
}

// buildUserPrompt 把 TopK chunks 拼成最终上下文。
// 优化理念：仅负责提供结构化的检索数据和用户问题，不包含任何逻辑指令，防止指令冗余和提示词注入。
func (aiService *AIService) buildUserPrompt(question string, hits []RAGSearchHit) string {
	var builder strings.Builder

	// 使用明确的 XML 标签包裹检索内容，让大模型清晰区分“参考资料”与“系统指令”
	builder.WriteString("<retrieved_blog_snippets>\n")

	if len(hits) == 0 {
		builder.WriteString("本次检索没有召回到相关的博客片段。\n")
	} else {
		for i, hit := range hits {
			builder.WriteString(fmt.Sprintf("--- 来源文章 [%d]: %s ---\n", i+1, hit.Title))
			builder.WriteString("片段内容：\n")
			builder.WriteString(hit.Text + "\n\n")
		}
	}
	builder.WriteString("</retrieved_blog_snippets>\n\n")

	// 简短的引导语，直接带出问题
	builder.WriteString("请结合上面的系统设定和检索片段，回答以下问题：\n")
	builder.WriteString(question)

	return builder.String()
}

// normalizeChatAnswer 对模型输出做最后一道轻量清洗。
// 目标不是重写答案，而是把裸露的 Markdown 标记和重复 URL 去掉，让聊天框读起来像正常回答。
func (aiService *AIService) normalizeChatAnswer(answer string) string {
	answer = strings.TrimSpace(answer)
	if answer == "" {
		return answer
	}

	markdownLinkRe := regexp.MustCompile(`\[[^\]]+\]\((https?://[^)\s]+)\)`)
	answer = markdownLinkRe.ReplaceAllStringFunc(answer, func(match string) string {
		end := strings.Index(match, "]")
		if end > 1 {
			return match[1:end]
		}
		return ""
	})
	rawURLRe := regexp.MustCompile(`https?://[^\s)）]+`)
	answer = rawURLRe.ReplaceAllString(answer, "")
	for _, token := range []string{"__"} {
		answer = strings.ReplaceAll(answer, token, "")
	}

	lines := strings.Split(answer, "\n")
	cleaned := make([]string, 0, len(lines))
	previousBlank := false
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			if !previousBlank && len(cleaned) > 0 {
				cleaned = append(cleaned, "")
			}
			previousBlank = true
			continue
		}
		cleaned = append(cleaned, line)
		previousBlank = false
	}
	return strings.TrimSpace(strings.Join(cleaned, "\n"))
}

// buildSources 从命中的 chunks 中提取去重后的文章来源。
// 多个 chunk 可能来自同一篇文章，前端只需要展示一次来源链接。
func (aiService *AIService) buildSources(hits []RAGSearchHit) []response.AIArticle {
	sources := make([]response.AIArticle, 0, len(hits))
	seen := make(map[string]struct{})
	for _, hit := range hits {
		key := hit.URL
		if key == "" {
			key = hit.ArticleID
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		sources = append(sources, response.AIArticle{
			ID:    hit.ArticleID,
			Title: hit.Title,
			URL:   hit.URL,
		})
	}
	return sources
}

// buildReferences 是第一版接口的来源构建逻辑。
// 第二版返回 sources，保留此函数是为了后续做关键词回退时快速接上。
func (aiService *AIService) buildReferences(articles []aiContextArticle) []response.AIArticle {
	references := make([]response.AIArticle, 0, len(articles))
	for _, article := range articles {
		references = append(references, response.AIArticle{
			ID:       article.ID,
			Title:    article.Source.Title,
			URL:      "/article/" + article.ID,
			Abstract: article.Source.Abstract,
			Category: article.Source.Category,
			Tags:     article.Source.Tags,
		})
	}
	return references
}

// articleSnippet 是第一版文章级检索时的正文摘要截取逻辑。
func (aiService *AIService) articleSnippet(article elasticsearch.Article, query string) string {
	content := strings.TrimSpace(stripMarkdown(article.Content))
	if content == "" {
		content = strings.TrimSpace(article.Abstract)
	}
	if content == "" {
		return ""
	}
	runes := []rune(content)
	queryRunes := []rune(strings.TrimSpace(query))
	if len(queryRunes) > 0 {
		index := strings.Index(content, string(queryRunes))
		if index >= 0 {
			startRune := utf8.RuneCountInString(content[:index]) - 300
			if startRune < 0 {
				startRune = 0
			}
			endRune := startRune + 1200
			if endRune > len(runes) {
				endRune = len(runes)
			}
			return strings.TrimSpace(string(runes[startRune:endRune]))
		}
	}
	return truncateRunes(content, 1200)
}

// notFoundAnswer 在 RAG 没召回任何 chunk 时返回兜底回答。
// 注意这里不调用大模型，避免在没有博客依据时产生编造内容。
func (aiService *AIService) notFoundAnswer(language string, question string) string {
	if strings.EqualFold(language, "en") || looksEnglish(question) {
		return "I could not find relevant information about this topic in the blog articles. Based on general knowledge, I can still give you a brief explanation: please try asking again with a more specific subject, and I will explain the topic while clearly separating general knowledge from blog-based references."
	}
	return "我没有在博客文章中找到足够相关的内容。就通用知识来说，你问到的这个问题仍然可以继续解释：我会先说明博客里没有对应资料，再根据公开常识补充背景、定义、关键特点和常见理解。你可以把问题问得更具体一些，例如询问某个人物是谁、某个技术概念怎么理解、某段历史有什么背景，我会把“博客依据”和“通用知识”分开说明，避免把博客里不存在的信息误说成博客内容。"
}

// stripMarkdown 是第一版用的轻量 Markdown 清洗函数。
// 第二版更完整的清洗逻辑在 rag.go 的 cleanArticleText 中。
func stripMarkdown(s string) string {
	replacements := []string{"```", "#", ">", "*", "`", "|"}
	for _, old := range replacements {
		s = strings.ReplaceAll(s, old, " ")
	}
	re := regexp.MustCompile(`!\[[^\]]*\]\([^)]+\)|\[[^\]]+\]\([^)]+\)`)
	s = re.ReplaceAllString(s, " ")
	re = regexp.MustCompile(`\s+`)
	return strings.TrimSpace(re.ReplaceAllString(s, " "))
}

// truncateRunes 按 Unicode 字符截断，中文不会被截成半个字节。
func truncateRunes(s string, limit int) string {
	runes := []rune(s)
	if limit <= 0 || len(runes) <= limit {
		return s
	}
	return string(runes[:limit]) + "..."
}

// looksEnglish 用一个简单规则判断用户是否主要使用英文。
// 它只用于兜底回答语言判断，不参与检索语言过滤。
func looksEnglish(s string) bool {
	var latin, cjk int
	for _, r := range s {
		switch {
		case r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z':
			latin++
		case r >= '\u4e00' && r <= '\u9fff':
			cjk++
		}
	}
	return latin > 0 && cjk == 0
}
