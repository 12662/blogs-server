package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"server/global"
	"server/model/elasticsearch"
)

type RAGService struct {
}

// RAGSearchHit 是在线检索阶段返回给 AIService 的轻量结构。
// 它只保留构建 Prompt 和前端引用所需的字段，不暴露完整 ES 命中文档。
type RAGSearchHit struct {
	ID        string
	ArticleID string
	Text      string
	Title     string
	URL       string
	Language  string
}

// embeddingRequest/embeddingResponse 对应通义千问 compatible-mode 的 /embeddings 接口。
// Input 一次最多按配置传 10 条，Dimensions 固定为 text-embedding-v4 的 1024 维。
type embeddingRequest struct {
	Model      string   `json:"model"`
	Input      []string `json:"input"`
	Dimensions int      `json:"dimensions,omitempty"`
}

type embeddingResponse struct {
	Data []struct {
		Index     int       `json:"index"`
		Embedding []float64 `json:"embedding"`
	} `json:"data"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
		Code    string `json:"code"`
	} `json:"error,omitempty"`
}

// esSearchResponse/esBulkResponse 是直接调用 ES HTTP JSON API 时用到的最小响应结构。
// 这里没有使用 typed client，是为了更直接地表达 ES8 的 retriever/rrf 查询 DSL。
type esSearchResponse struct {
	Hits struct {
		Hits []struct {
			ID     string          `json:"_id"`
			Source json.RawMessage `json:"_source"`
		} `json:"hits"`
		Total struct {
			Value int64 `json:"value"`
		} `json:"total"`
	} `json:"hits"`
}

type esBulkResponse struct {
	Errors bool `json:"errors"`
	Items  []map[string]struct {
		Error *struct {
			Type   string `json:"type"`
			Reason string `json:"reason"`
		} `json:"error,omitempty"`
	} `json:"items"`
}

// ChunkIndex 读取当前 RAG 切片索引名。
// 默认值是 rag_chunks，但保留配置项后，后续灰度重建索引时可以改成 rag_chunks_v2。
func (ragService *RAGService) ChunkIndex() string {
	if global.Config.AI.RAGIndex != "" {
		return global.Config.AI.RAGIndex
	}
	return elasticsearch.RagChunkIndex()
}

// EnsureChunkIndex 确保 rag_chunks 索引存在。
// content_vector 是 dense_vector，开启 index=true 后才能做 ES8 knn 检索。
// similarity=cosine 表示使用余弦相似度，适合文本向量相似度搜索。
func (ragService *RAGService) EnsureChunkIndex(ctx context.Context) error {
	exists, err := ragService.indexExists(ctx, ragService.ChunkIndex())
	if err != nil {
		return err
	}
	if exists {
		return nil
	}
	dims := global.Config.AI.EmbeddingDimensions
	if dims <= 0 {
		dims = 1024
	}
	mapping := map[string]any{
		"mappings": map[string]any{
			"properties": map[string]any{
				"id":         map[string]any{"type": "keyword"},
				"article_id": map[string]any{"type": "keyword"},
				"content_vector": map[string]any{
					"type":       "dense_vector",
					"dims":       dims,
					"index":      true,
					"similarity": "cosine",
				},
				"text":     map[string]any{"type": "text"},
				"title":    map[string]any{"type": "text", "fields": map[string]any{"keyword": map[string]any{"type": "keyword", "ignore_above": 256}}},
				"url":      map[string]any{"type": "keyword"},
				"language": map[string]any{"type": "keyword"},
			},
		},
	}
	var result map[string]any
	return ragService.doES(ctx, http.MethodPut, "/"+ragService.ChunkIndex(), mapping, &result)
}

// SyncArticle 是文章发布/更新后的增量同步入口。
// 当前策略是先删除该文章旧 chunks，再根据最新文章内容重新切片并写入。
// 这样实现简单且结果一致，适合博客文章这种更新频率不高的场景。
func (ragService *RAGService) SyncArticle(ctx context.Context, articleID string, article elasticsearch.Article) (int, string, error) {
	if strings.TrimSpace(articleID) == "" {
		return 0, "", errors.New("article id is required")
	}
	if err := ragService.EnsureChunkIndex(ctx); err != nil {
		return 0, "", err
	}
	if err := ragService.DeleteArticleChunks(ctx, articleID); err != nil {
		return 0, "", err
	}
	contentHash := ragService.ArticleContentHash(article)
	chunks := ragService.BuildChunks(articleID, article)
	if len(chunks) == 0 {
		return 0, contentHash, nil
	}
	texts := make([]string, 0, len(chunks))
	for _, chunk := range chunks {
		texts = append(texts, chunk.Text)
	}
	vectors, err := ragService.EmbedTexts(ctx, texts)
	if err != nil {
		return 0, "", err
	}
	if len(vectors) != len(chunks) {
		return 0, "", fmt.Errorf("embedding count mismatch: got %d want %d", len(vectors), len(chunks))
	}
	for i := range chunks {
		chunks[i].ContentVector = vectors[i]
	}
	if err := ragService.BulkUpsertChunks(ctx, chunks); err != nil {
		return 0, "", err
	}
	return len(chunks), contentHash, nil
}

// IngestAllArticles 是历史数据全量导入入口。
// 它分页扫描 article_index，逐篇文章调用 IngestArticleChunks。
// 如果文章很多，可以后续扩展为从上次游标继续、或按更新时间增量导入。
func (ragService *RAGService) IngestAllArticles(ctx context.Context) error {
	if err := ragService.EnsureChunkIndex(ctx); err != nil {
		return err
	}
	const pageSize = 100
	for from := 0; ; from += pageSize {
		body := map[string]any{
			"from": from,
			"size": pageSize,
			"query": map[string]any{
				"match_all": map[string]any{},
			},
			"_source": []string{"title", "abstract", "content"},
		}
		var result esSearchResponse
		if err := ragService.doES(ctx, http.MethodPost, "/"+elasticsearch.ArticleIndex()+"/_search", body, &result); err != nil {
			return err
		}
		if len(result.Hits.Hits) == 0 {
			break
		}
		for _, hit := range result.Hits.Hits {
			var article elasticsearch.Article
			if err := json.Unmarshal(hit.Source, &article); err != nil {
				return err
			}
			chunkCount, contentHash, err := ragService.IngestArticleChunks(ctx, hit.ID, article)
			if err != nil {
				return err
			}
			_ = ServiceGroupApp.ArticleService.UpdateRAGSyncSuccess(hit.ID, chunkCount, contentHash)
		}
		if len(result.Hits.Hits) < pageSize {
			break
		}
	}
	return nil
}

// IngestArticleChunks 用于全量导入时处理单篇文章。
// 与 SyncArticle 的主要区别是这里使用并发 Embedding，适合一次性处理大量历史文章。
func (ragService *RAGService) IngestArticleChunks(ctx context.Context, articleID string, article elasticsearch.Article) (int, string, error) {
	if strings.TrimSpace(articleID) == "" {
		return 0, "", nil
	}
	if err := ragService.DeleteArticleChunks(ctx, articleID); err != nil {
		return 0, "", err
	}
	contentHash := ragService.ArticleContentHash(article)
	chunks := ragService.BuildChunks(articleID, article)
	if len(chunks) == 0 {
		return 0, contentHash, nil
	}
	texts := make([]string, 0, len(chunks))
	for _, chunk := range chunks {
		texts = append(texts, chunk.Text)
	}
	vectors, err := ragService.EmbedTextsConcurrent(ctx, texts)
	if err != nil {
		return 0, "", err
	}
	if len(vectors) != len(chunks) {
		return 0, "", fmt.Errorf("embedding count mismatch: got %d want %d", len(vectors), len(chunks))
	}
	for i := range chunks {
		chunks[i].ContentVector = vectors[i]
	}
	if err := ragService.BulkUpsertChunks(ctx, chunks); err != nil {
		return 0, "", err
	}
	return len(chunks), contentHash, nil
}

// DeleteArticleChunks 按 article_id 删除某篇文章生成的所有向量切片。
// 文章删除或下线时调用它，保证 rag_chunks 不残留过期知识。
func (ragService *RAGService) DeleteArticleChunks(ctx context.Context, articleID string) error {
	if strings.TrimSpace(articleID) == "" {
		return nil
	}
	exists, err := ragService.indexExists(ctx, ragService.ChunkIndex())
	if err != nil || !exists {
		return err
	}
	body := map[string]any{
		"query": map[string]any{
			"term": map[string]any{
				"article_id": articleID,
			},
		},
	}
	var result map[string]any
	return ragService.doES(ctx, http.MethodPost, "/"+ragService.ChunkIndex()+"/_delete_by_query?refresh=true&conflicts=proceed", body, &result)
}

// BuildChunks 把一篇文章转成若干 RagChunk 元数据。
// 这里只负责切片和生成稳定 ID，不在这里调用 Embedding API，方便单独测试切片效果。
func (ragService *RAGService) BuildChunks(articleID string, article elasticsearch.Article) []elasticsearch.RagChunk {
	urlValue := ragService.ArticleURL(articleID)
	title := truncateRunes(strings.TrimSpace(article.Title), 100)
	rawText := strings.TrimSpace(article.Content)
	if rawText == "" {
		rawText = strings.TrimSpace(article.Abstract)
	}
	parts := splitMarkdownIntoChunks(rawText, 800, 500)
	chunks := make([]elasticsearch.RagChunk, 0, len(parts))
	hashPrefix := hashURLPrefix(urlValue)
	for i, part := range parts {
		part = truncateRunes(strings.TrimSpace(part), 800)
		if part == "" {
			continue
		}
		chunkID := fmt.Sprintf("%s-%d", hashPrefix, i)
		chunks = append(chunks, elasticsearch.RagChunk{
			ID:        chunkID,
			ArticleID: articleID,
			Text:      part,
			Title:     title,
			URL:       urlValue,
			Language:  "zh",
		})
	}
	return chunks
}

// ArticleURL 生成文章的规范 URL。
// 线上部署时需要把 ai.article_base_url 改成真实域名，例如 https://example.com/article。
func (ragService *RAGService) ArticleURL(articleID string) string {
	baseURL := strings.TrimRight(global.Config.AI.ArticleBaseURL, "/")
	if baseURL == "" {
		baseURL = "http://localhost/article"
	}
	return baseURL + "/" + url.PathEscape(articleID)
}

// EmbedTexts 是串行批量向量化入口。
// 在线聊天查询和文章发布/更新使用它，避免并发过高影响接口响应稳定性。
func (ragService *RAGService) EmbedTexts(ctx context.Context, texts []string) ([][]float64, error) {
	if len(texts) == 0 {
		return [][]float64{}, nil
	}
	batchSize := global.Config.AI.EmbeddingBatchSize
	if batchSize <= 0 || batchSize > 10 {
		batchSize = 10
	}
	vectors := make([][]float64, len(texts))
	for start := 0; start < len(texts); start += batchSize {
		end := start + batchSize
		if end > len(texts) {
			end = len(texts)
		}
		batchVectors, err := ragService.embedBatchWithRetry(ctx, texts[start:end])
		if err != nil {
			return nil, err
		}
		if len(batchVectors) != end-start {
			return nil, fmt.Errorf("embedding batch count mismatch: got %d want %d", len(batchVectors), end-start)
		}
		copy(vectors[start:end], batchVectors)
	}
	return vectors, nil
}

// EmbedTextsConcurrent 是全量导入时使用的并发向量化入口。
// 每个 batch 最多 10 条，符合模型限制；并发数由 ai.embedding_concurrency 控制。
func (ragService *RAGService) EmbedTextsConcurrent(ctx context.Context, texts []string) ([][]float64, error) {
	if len(texts) == 0 {
		return [][]float64{}, nil
	}
	batchSize := global.Config.AI.EmbeddingBatchSize
	if batchSize <= 0 || batchSize > 10 {
		batchSize = 10
	}
	concurrency := global.Config.AI.EmbeddingConcurrency
	if concurrency <= 0 {
		concurrency = 2
	}
	type job struct {
		start int
		end   int
	}
	jobs := make(chan job)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var firstErr error
	vectors := make([][]float64, len(texts))

	worker := func() {
		defer wg.Done()
		for item := range jobs {
			if ctx.Err() != nil {
				return
			}
			mu.Lock()
			hasErr := firstErr != nil
			mu.Unlock()
			if hasErr {
				continue
			}
			batchVectors, err := ragService.embedBatchWithRetry(ctx, texts[item.start:item.end])
			if err != nil {
				mu.Lock()
				if firstErr == nil {
					firstErr = err
				}
				mu.Unlock()
				return
			}
			mu.Lock()
			copy(vectors[item.start:item.end], batchVectors)
			mu.Unlock()
		}
	}

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go worker()
	}
	for start := 0; start < len(texts); start += batchSize {
		end := start + batchSize
		if end > len(texts) {
			end = len(texts)
		}
		jobs <- job{start: start, end: end}
	}
	close(jobs)
	wg.Wait()
	if firstErr != nil {
		return nil, firstErr
	}
	return vectors, nil
}

// BulkUpsertChunks 使用 ES Bulk API 批量写入切片。
// 这里用 index 操作配合稳定 _id，重复导入同一个 chunk 会覆盖旧文档。
func (ragService *RAGService) BulkUpsertChunks(ctx context.Context, chunks []elasticsearch.RagChunk) error {
	if len(chunks) == 0 {
		return nil
	}
	var builder strings.Builder
	for _, chunk := range chunks {
		meta, _ := json.Marshal(map[string]any{"index": map[string]any{"_index": ragService.ChunkIndex(), "_id": chunk.ID}})
		doc, err := json.Marshal(chunk)
		if err != nil {
			return err
		}
		builder.Write(meta)
		builder.WriteByte('\n')
		builder.Write(doc)
		builder.WriteByte('\n')
	}
	var result esBulkResponse
	if err := ragService.doESRaw(ctx, http.MethodPost, "/_bulk?refresh=true", "application/x-ndjson", strings.NewReader(builder.String()), &result); err != nil {
		return err
	}
	if result.Errors {
		for _, item := range result.Items {
			for _, detail := range item {
				if detail.Error != nil {
					return fmt.Errorf("bulk upsert chunk failed: %s: %s", detail.Error.Type, detail.Error.Reason)
				}
			}
		}
		return errors.New("bulk upsert chunk failed")
	}
	return nil
}

// HybridSearch 是在线问答的核心检索逻辑。
// 流程：问题向量化 -> ES 同时执行 match 检索和 knn 检索 -> 使用 RRF 融合排名 -> 返回 TopK chunks。
func (ragService *RAGService) HybridSearch(ctx context.Context, message string, language string, topK int) ([]RAGSearchHit, error) {
	if topK <= 0 {
		topK = 8
	}
	if strings.TrimSpace(language) == "" {
		language = "zh"
	}
	vectors, err := ragService.EmbedTexts(ctx, []string{message})
	if err != nil {
		return nil, err
	}
	if len(vectors) == 0 {
		return nil, errors.New("query embedding is empty")
	}
	body := map[string]any{
		"size": topK,
		"_source": []string{
			"article_id", "text", "title", "url", "language",
		},
		"retriever": map[string]any{
			"rrf": map[string]any{
				"rank_window_size": 50,
				"rank_constant":    60,
				"retrievers": []any{
					map[string]any{
						"standard": map[string]any{
							"query": map[string]any{
								"bool": map[string]any{
									"must": map[string]any{
										"match": map[string]any{"text": message},
									},
									"filter": []any{
										map[string]any{"term": map[string]any{"language": language}},
									},
								},
							},
						},
					},
					map[string]any{
						"knn": map[string]any{
							"field":          "content_vector",
							"query_vector":   vectors[0],
							"k":              50,
							"num_candidates": 100,
							"filter": []any{
								map[string]any{"term": map[string]any{"language": language}},
							},
						},
					},
				},
			},
		},
	}
	var result esSearchResponse
	if err := ragService.doES(ctx, http.MethodPost, "/"+ragService.ChunkIndex()+"/_search", body, &result); err != nil {
		return nil, err
	}
	return ragService.parseChunkHits(result), nil
}

// KeywordSearchChunks 是 Hybrid Search 的第一层兜底。
// 如果查询向量化失败、ES knn 暂时不可用，仍然可以靠 rag_chunks 的 text/title 倒排索引召回内容。
func (ragService *RAGService) KeywordSearchChunks(ctx context.Context, message string, language string, topK int) ([]RAGSearchHit, error) {
	if topK <= 0 {
		topK = 8
	}
	if strings.TrimSpace(language) == "" {
		language = "zh"
	}
	body := map[string]any{
		"size": topK,
		"_source": []string{
			"article_id", "text", "title", "url", "language",
		},
		"query": map[string]any{
			"bool": map[string]any{
				"should": []any{
					map[string]any{"match": map[string]any{"text": message}},
					map[string]any{"match": map[string]any{"title": message}},
				},
				"minimum_should_match": 1,
				"filter": []any{
					map[string]any{"term": map[string]any{"language": language}},
				},
			},
		},
	}
	var result esSearchResponse
	if err := ragService.doES(ctx, http.MethodPost, "/"+ragService.ChunkIndex()+"/_search", body, &result); err != nil {
		return nil, err
	}
	return ragService.parseChunkHits(result), nil
}

func (ragService *RAGService) parseChunkHits(result esSearchResponse) []RAGSearchHit {
	hits := make([]RAGSearchHit, 0, len(result.Hits.Hits))
	for _, hit := range result.Hits.Hits {
		var chunk elasticsearch.RagChunk
		if err := json.Unmarshal(hit.Source, &chunk); err != nil {
			continue
		}
		hits = append(hits, RAGSearchHit{
			ID:        hit.ID,
			ArticleID: chunk.ArticleID,
			Text:      chunk.Text,
			Title:     chunk.Title,
			URL:       chunk.URL,
			Language:  chunk.Language,
		})
	}
	return hits
}

// embedBatchWithRetry 给 Embedding API 加指数退避重试。
// 网络抖动或限流时，最多尝试 3 次，等待时间依次放大。
func (ragService *RAGService) embedBatchWithRetry(ctx context.Context, texts []string) ([][]float64, error) {
	var lastErr error
	backoff := 500 * time.Millisecond
	for attempt := 0; attempt < 3; attempt++ {
		vectors, err := ragService.embedBatch(ctx, texts)
		if err == nil {
			return vectors, nil
		}
		lastErr = err
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(backoff):
		}
		backoff *= 2
	}
	return nil, lastErr
}

// embedBatch 真正调用千问 Embedding API。
// 返回结果会按接口中的 index 放回原顺序，保证向量和文本切片一一对应。
func (ragService *RAGService) embedBatch(ctx context.Context, texts []string) ([][]float64, error) {
	apiKey := strings.TrimSpace(global.Config.AI.QwenAPIKey)
	if apiKey == "" {
		return nil, errors.New("qwen api key is not configured")
	}
	baseURL := strings.TrimRight(global.Config.AI.QwenBaseURL, "/")
	if baseURL == "" {
		baseURL = "https://dashscope.aliyuncs.com/compatible-mode/v1"
	}
	model := global.Config.AI.QwenEmbeddingModel
	if model == "" {
		model = "text-embedding-v4"
	}
	dims := global.Config.AI.EmbeddingDimensions
	if dims <= 0 {
		dims = 1024
	}
	timeout := global.Config.AI.RequestTimeout
	if timeout <= 0 {
		timeout = 60
	}
	ctx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
	defer cancel()

	body, err := json.Marshal(embeddingRequest{Model: model, Input: texts, Dimensions: dims})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/embeddings", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var parsed embeddingResponse
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if parsed.Error != nil && parsed.Error.Message != "" {
			return nil, errors.New(parsed.Error.Message)
		}
		return nil, fmt.Errorf("embedding request failed: %s", resp.Status)
	}
	vectors := make([][]float64, len(texts))
	for _, item := range parsed.Data {
		if item.Index >= 0 && item.Index < len(vectors) {
			vectors[item.Index] = item.Embedding
		}
	}
	for i, vector := range vectors {
		if len(vector) != dims {
			return nil, fmt.Errorf("embedding %d has %d dimensions, want %d", i, len(vector), dims)
		}
	}
	return vectors, nil
}

// indexExists 用 HEAD /index 判断索引是否已经存在。
// 因为 rag_chunks 使用原生 JSON DSL 创建，这里也用 HTTP API 保持一致。
func (ragService *RAGService) indexExists(ctx context.Context, index string) (bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, strings.TrimRight(global.Config.ES.URL, "/")+"/"+index, nil)
	if err != nil {
		return false, err
	}
	if global.Config.ES.Username != "" {
		req.SetBasicAuth(global.Config.ES.Username, global.Config.ES.Password)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return false, nil
	}
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return true, nil
	}
	return false, fmt.Errorf("check index exists failed: %s", resp.Status)
}

// doES 把普通 Go map 编码成 JSON，然后发送到 Elasticsearch。
func (ragService *RAGService) doES(ctx context.Context, method string, path string, body any, result any) error {
	var reader io.Reader
	if body != nil {
		bytesBody, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(bytesBody)
	}
	return ragService.doESRaw(ctx, method, path, "application/json", reader, result)
}

// doESRaw 是更底层的 ES HTTP 调用函数。
// Bulk API 需要 application/x-ndjson，所以这里允许调用者传入 contentType 和原始 body。
func (ragService *RAGService) doESRaw(ctx context.Context, method string, path string, contentType string, body io.Reader, result any) error {
	endpoint := strings.TrimRight(global.Config.ES.URL, "/") + path
	req, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		return err
	}
	if contentType != "" && body != nil {
		req.Header.Set("Content-Type", contentType)
	}
	if global.Config.ES.Username != "" {
		req.SetBasicAuth(global.Config.ES.Username, global.Config.ES.Password)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("elasticsearch request failed: %s: %s", resp.Status, string(respBody))
	}
	if result != nil && len(respBody) > 0 {
		return json.Unmarshal(respBody, result)
	}
	return nil
}

// hashURLPrefix 取 sha256(url) 的前 12 位，作为 chunk ID 的稳定前缀。
func hashURLPrefix(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])[:12]
}

// ArticleContentHash 计算当前文章可用于 RAG 的内容指纹。
// 定时维护任务用它和 article_index 里的 rag_content_hash 对比，判断向量是否过期。
func (ragService *RAGService) ArticleContentHash(article elasticsearch.Article) string {
	parts := []string{
		strings.TrimSpace(article.Title),
		strings.TrimSpace(article.Abstract),
		cleanArticleText(article.Content),
	}
	sum := sha256.Sum256([]byte(strings.Join(parts, "\n")))
	return hex.EncodeToString(sum[:])
}

// cleanArticleText 把 Markdown/HTML 内容清洗成适合向量化的纯文本。
// 这里尽量保留正文语义，去掉代码块、标签、图片链接和多余空白。
func cleanArticleText(text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = regexp.MustCompile("(?s)```.*?```").ReplaceAllString(text, " ")
	text = regexp.MustCompile("(?s)<script.*?</script>|<style.*?</style>").ReplaceAllString(text, " ")
	text = regexp.MustCompile("<[^>]+>").ReplaceAllString(text, " ")
	text = regexp.MustCompile(`!\[[^\]]*]\([^)]+\)`).ReplaceAllString(text, " ")
	text = regexp.MustCompile(`\[([^\]]+)]\([^)]+\)`).ReplaceAllString(text, "$1")
	text = regexp.MustCompile(`(?m)^#{1,6}\s*`).ReplaceAllString(text, "\n")
	for _, mark := range []string{"*", "_", "`", ">", "|", "~"} {
		text = strings.ReplaceAll(text, mark, " ")
	}
	text = regexp.MustCompile(`\s+`).ReplaceAllString(text, " ")
	return strings.TrimSpace(text)
}

// splitMarkdownIntoChunks 先按 Markdown 标题分段，再对超长段落做句子级切分。
// 这是离线入库的主切片函数。
func splitMarkdownIntoChunks(markdown string, maxChars int, minChars int) []string {
	sections := splitByMarkdownHeadings(markdown)
	var chunks []string
	for _, section := range sections {
		cleaned := cleanArticleText(section)
		if cleaned == "" {
			continue
		}
		if runeLen(cleaned) <= maxChars {
			chunks = append(chunks, cleaned)
			continue
		}
		chunks = append(chunks, splitBySentence(cleaned, maxChars, minChars)...)
	}
	return chunks
}

// splitTextIntoChunks 是一个更通用的纯文本切片函数，目前保留给后续测试或复用。
func splitTextIntoChunks(text string, maxChars int, minChars int) []string {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	if maxChars <= 0 {
		maxChars = 800
	}
	sections := splitByMarkdownHeadings(text)
	var chunks []string
	for _, section := range sections {
		section = strings.TrimSpace(section)
		if section == "" {
			continue
		}
		if runeLen(section) <= maxChars {
			chunks = append(chunks, section)
			continue
		}
		chunks = append(chunks, splitBySentence(section, maxChars, minChars)...)
	}
	return chunks
}

// splitByMarkdownHeadings 按 #、##、### 等 Markdown 标题边界拆分文章。
// 标题通常代表语义章节，用它作为第一层切分可以让 chunk 更完整。
func splitByMarkdownHeadings(text string) []string {
	lines := strings.Split(text, "\n")
	var sections []string
	var builder strings.Builder
	for _, line := range lines {
		if regexp.MustCompile(`^#{1,6}\s+`).MatchString(strings.TrimSpace(line)) && builder.Len() > 0 {
			sections = append(sections, builder.String())
			builder.Reset()
		}
		builder.WriteString(line)
		builder.WriteByte('\n')
	}
	if builder.Len() > 0 {
		sections = append(sections, builder.String())
	}
	if len(sections) == 0 {
		return []string{text}
	}
	return sections
}

// splitBySentence 对仍然过长的章节按句末标点拆分，并尽量凑到 minChars 后再成块。
func splitBySentence(text string, maxChars int, minChars int) []string {
	sentences := regexp.MustCompile(`[^。！？!?；;]+[。！？!?；;]?`).FindAllString(text, -1)
	if len(sentences) == 0 {
		return hardSplit(text, maxChars)
	}
	var chunks []string
	var builder strings.Builder
	for _, sentence := range sentences {
		sentence = strings.TrimSpace(sentence)
		if sentence == "" {
			continue
		}
		if runeLen(sentence) > maxChars {
			if builder.Len() > 0 {
				chunks = append(chunks, strings.TrimSpace(builder.String()))
				builder.Reset()
			}
			chunks = append(chunks, hardSplit(sentence, maxChars)...)
			continue
		}
		nextLen := runeLen(builder.String()) + runeLen(sentence)
		if builder.Len() > 0 && nextLen > maxChars {
			chunks = append(chunks, strings.TrimSpace(builder.String()))
			builder.Reset()
		}
		builder.WriteString(sentence)
		if runeLen(builder.String()) >= minChars {
			chunks = append(chunks, strings.TrimSpace(builder.String()))
			builder.Reset()
		}
	}
	if builder.Len() > 0 {
		chunks = append(chunks, strings.TrimSpace(builder.String()))
	}
	return chunks
}

// hardSplit 是最后兜底：如果单句仍然超过 maxChars，只能按 rune 数硬切。
func hardSplit(text string, maxChars int) []string {
	runes := []rune(text)
	var chunks []string
	for start := 0; start < len(runes); start += maxChars {
		end := start + maxChars
		if end > len(runes) {
			end = len(runes)
		}
		chunks = append(chunks, strings.TrimSpace(string(runes[start:end])))
	}
	return chunks
}

// runeLen 按 Unicode 字符计数，避免中文被按字节长度误判。
func runeLen(text string) int {
	return len([]rune(text))
}
