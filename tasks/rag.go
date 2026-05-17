package tasks

import (
	"encoding/json"

	"github.com/hibiken/asynq"
)

const (
	// TypeSyncRAG 是 Asynq 中的任务类型名。
	// Worker 会用这个字符串把任务路由到 HandleSyncRAGTask。
	TypeSyncRAG = "rag:sync"

	// QueueRAG 单独放 RAG 任务，避免向量同步任务挤占其他后台任务。
	QueueRAG = "rag"
)

type SyncRAGPayload struct {
	ArticleID string `json:"article_id"`
}

// NewSyncRAGTask 封装“同步某篇文章向量”的任务。
// 任务 Payload 只放 article_id，Worker 执行时再读取最新文章内容，避免队列里保存过期正文。
func NewSyncRAGTask(articleID string) (*asynq.Task, error) {
	payload, err := json.Marshal(SyncRAGPayload{ArticleID: articleID})
	if err != nil {
		return nil, err
	}
	return asynq.NewTask(TypeSyncRAG, payload), nil
}
