package worker

import (
	"context"
	"encoding/json"
	"fmt"

	"server/model/elasticsearch"
	"server/service"
	"server/tasks"

	"github.com/hibiken/asynq"
)

// HandleSyncRAGTask 是异步向量同步任务的处理器。
//
// 当前仓库的文章正文存在 ES article_index 中，所以这里通过 ArticleService.Get 读取最新文章。
// 如果后续改成 MySQL articles 表，可以把这里替换成 GORM 查询，例如：
//
//	var article database.Article
//	err := global.DB.Where("id = ?", payload.ArticleID).First(&article).Error
//
// 任务失败时返回 error 给 Asynq，Asynq 会按 MaxRetry(3) 自动重试。
func HandleSyncRAGTask(ctx context.Context, task *asynq.Task) error {
	var payload tasks.SyncRAGPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return fmt.Errorf("unmarshal sync rag payload: %w", err)
	}
	if payload.ArticleID == "" {
		return fmt.Errorf("article_id is required")
	}

	articleService := service.ServiceGroupApp.ArticleService

	// 任务开始时保持 pending。这个更新是幂等的，重复执行不会影响业务数据。
	_ = articleService.UpdateRAGSyncStatus(payload.ArticleID, elasticsearch.RAGSyncStatusPending, "")

	article, err := articleService.Get(payload.ArticleID)
	if err != nil {
		return markFailedIfLastRetry(ctx, payload.ArticleID, fmt.Errorf("get article: %w", err))
	}

	chunkCount, contentHash, err := service.ServiceGroupApp.RAGService.SyncArticle(ctx, payload.ArticleID, article)
	if err != nil {
		return markFailedIfLastRetry(ctx, payload.ArticleID, err)
	}

	return articleService.UpdateRAGSyncSuccess(payload.ArticleID, chunkCount, contentHash)
}

func markFailedIfLastRetry(ctx context.Context, articleID string, err error) error {
	retryCount, _ := asynq.GetRetryCount(ctx)
	maxRetry, _ := asynq.GetMaxRetry(ctx)
	if retryCount >= maxRetry {
		// 这是最后一次失败，状态改为 failed，方便后台管理台筛选和手动重试。
		_ = service.ServiceGroupApp.ArticleService.UpdateRAGSyncStatus(articleID, elasticsearch.RAGSyncStatusFailed, err.Error())
	}
	return err
}
