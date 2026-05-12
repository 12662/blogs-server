package service

import (
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

	"server/global"
	"server/model/elasticsearch"
	"server/model/request"
	"server/model/response"

	"github.com/elastic/go-elasticsearch/v8/typedapi/core/search"
	"github.com/elastic/go-elasticsearch/v8/typedapi/types"
)

type AIService struct {
}

type aiContextArticle struct {
	ID      string
	Source  elasticsearch.Article
	Snippet string
}

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
		Message qwenChatMessage `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
		Code    string `json:"code"`
	} `json:"error,omitempty"`
}

func (aiService *AIService) Chat(req request.AIChat) (response.AIChat, error) {
	question := strings.TrimSpace(req.Message)
	if question == "" {
		return response.AIChat{}, errors.New("message is required")
	}
	if !global.Config.AI.Enable {
		return response.AIChat{}, errors.New("AI chat is disabled")
	}
	articles, err := aiService.searchArticles(context.TODO(), question)
	if err != nil {
		return response.AIChat{}, err
	}
	references := aiService.buildReferences(articles)
	if len(articles) == 0 {
		return response.AIChat{
			Answer:     aiService.notFoundAnswer(req.Language, question),
			References: references,
		}, nil
	}
	answer, err := aiService.callQwen(context.TODO(), req, articles)
	if err != nil {
		return response.AIChat{}, err
	}
	return response.AIChat{
		Answer:     strings.TrimSpace(answer),
		References: references,
	}, nil
}

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

func (aiService *AIService) callQwen(ctx context.Context, req request.AIChat, articles []aiContextArticle) (string, error) {
	apiKey := strings.TrimSpace(global.Config.AI.QwenAPIKey)
	if apiKey == "" {
		return "", errors.New("qwen api key is not configured")
	}
	baseURL := strings.TrimRight(global.Config.AI.QwenBaseURL, "/")
	if baseURL == "" {
		baseURL = "https://dashscope.aliyuncs.com/compatible-mode/v1"
	}
	model := global.Config.AI.QwenModel
	if model == "" {
		model = "qwen-turbo-latest"
	}
	timeout := global.Config.AI.RequestTimeout
	if timeout <= 0 {
		timeout = 60
	}
	ctx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
	defer cancel()

	payload := qwenChatRequest{
		Model:       model,
		Messages:    aiService.buildMessages(req, articles),
		Temperature: global.Config.AI.Temperature,
		MaxTokens:   global.Config.AI.MaxTokens,
		Stream:      false,
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
	return qwenResp.Choices[0].Message.Content, nil
}

func (aiService *AIService) buildMessages(req request.AIChat, articles []aiContextArticle) []qwenChatMessage {
	languageRule := "请使用与用户最后一个问题相同的语言回答；如果用户使用英文提问，请使用英文回答。"
	if strings.EqualFold(req.Language, "en") {
		languageRule = "Answer in English because the user is asking in English."
	}
	systemPrompt := "你是赵天奇的博客的 AI 助手。只能根据提供的博客文章内容回答问题，不要编造博客中不存在的信息。回答要简洁、准确，并在合适时提到参考文章标题。" + languageRule
	messages := []qwenChatMessage{{Role: "system", Content: systemPrompt}}

	history := req.History
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

	userPrompt := aiService.buildUserPrompt(strings.TrimSpace(req.Message), articles)
	messages = append(messages, qwenChatMessage{Role: "user", Content: userPrompt})
	return messages
}

func (aiService *AIService) buildUserPrompt(question string, articles []aiContextArticle) string {
	var builder strings.Builder
	builder.WriteString("请根据下面的博客文章内容回答用户问题。\n")
	builder.WriteString("如果文章内容不足以回答，请明确说明没有在博客中找到足够依据。\n\n")
	builder.WriteString("博客文章内容：\n")
	for i, article := range articles {
		builder.WriteString(fmt.Sprintf("\n[%d] 标题：%s\n", i+1, article.Source.Title))
		if article.Source.Abstract != "" {
			builder.WriteString("摘要：" + article.Source.Abstract + "\n")
		}
		if article.Source.Category != "" {
			builder.WriteString("分类：" + article.Source.Category + "\n")
		}
		if len(article.Source.Tags) > 0 {
			builder.WriteString("标签：" + strings.Join(article.Source.Tags, "、") + "\n")
		}
		builder.WriteString("内容片段：" + article.Snippet + "\n")
	}
	builder.WriteString("\n用户问题：\n")
	builder.WriteString(question)
	return builder.String()
}

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

func (aiService *AIService) notFoundAnswer(language string, question string) string {
	if strings.EqualFold(language, "en") || looksEnglish(question) {
		return "I could not find enough relevant content in the blog articles to answer this question."
	}
	return "我没有在博客文章中找到足够相关的内容，暂时无法基于博客给出可靠回答。"
}

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

func truncateRunes(s string, limit int) string {
	runes := []rune(s)
	if limit <= 0 || len(runes) <= limit {
		return s
	}
	return string(runes[:limit]) + "..."
}

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
