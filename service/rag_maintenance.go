package service

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"server/global"
	"server/model/elasticsearch"
)

type RAGMaintenanceResult struct {
	Scanned int `json:"scanned"`
	Queued  int `json:"queued"`
	Skipped int `json:"skipped"`
	Failed  int `json:"failed"`
}

// MaintainArticles 扫描已发布文章，发现缺失或过期的向量后只投递异步任务。
// 这里不直接调用千问 Embedding API，避免定时任务阻塞太久，也避免和主业务抢资源。
func (ragService *RAGService) MaintainArticles(ctx context.Context) (RAGMaintenanceResult, error) {
	result := RAGMaintenanceResult{}
	if !global.Config.AI.Enable {
		return result, nil
	}
	if err := ragService.EnsureChunkIndex(ctx); err != nil {
		return result, err
	}

	pageSize := global.Config.AI.RAGMaintenanceBatchSize
	if pageSize <= 0 {
		pageSize = 50
	}

	for from := 0; ; from += pageSize {
		body := map[string]any{
			"from": from,
			"size": pageSize,
			"query": map[string]any{
				"match_all": map[string]any{},
			},
			"_source": []string{
				"title",
				"abstract",
				"content",
				"updated_at",
				"rag_sync_status",
				"rag_sync_error",
				"rag_chunk_count",
				"rag_content_hash",
				"rag_synced_at",
			},
		}

		var searchResult esSearchResponse
		if err := ragService.doES(ctx, http.MethodPost, "/"+elasticsearch.ArticleIndex()+"/_search", body, &searchResult); err != nil {
			return result, err
		}
		if len(searchResult.Hits.Hits) == 0 {
			break
		}

		for _, hit := range searchResult.Hits.Hits {
			result.Scanned++
			var article elasticsearch.Article
			if err := json.Unmarshal(hit.Source, &article); err != nil {
				result.Failed++
				continue
			}

			if !ragService.ShouldSyncArticle(article) {
				result.Skipped++
				continue
			}

			// RetryRAGSync 会把状态重置为 pending，并把 SyncRAGTask 投递到 Asynq 队列。
			if err := ServiceGroupApp.ArticleService.RetryRAGSync(ctx, hit.ID); err != nil {
				result.Failed++
				continue
			}
			result.Queued++
		}

		if len(searchResult.Hits.Hits) < pageSize {
			break
		}
	}
	return result, nil
}

// ShouldSyncArticle 判断当前文章是否需要重新生成向量。
// 规则按保守顺序执行：pending 不重复排队；untracked 是人工退出追踪；failed/空状态/内容变化/切片数量变化都重新排队。
func (ragService *RAGService) ShouldSyncArticle(article elasticsearch.Article) bool {
	status := strings.TrimSpace(article.RAGSyncStatus)
	if status == elasticsearch.RAGSyncStatusPending {
		return false
	}
	if status == elasticsearch.RAGSyncStatusUntracked {
		return false
	}

	chunks := ragService.BuildChunks("", article)
	if len(chunks) == 0 {
		return false
	}
	if status == "" || status == elasticsearch.RAGSyncStatusFailed {
		return true
	}
	if status != elasticsearch.RAGSyncStatusSuccess {
		return true
	}
	if article.RAGChunkCount != len(chunks) {
		return true
	}
	return strings.TrimSpace(article.RAGContentHash) != ragService.ArticleContentHash(article)
}
