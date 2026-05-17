package task

import (
	"context"
	"time"

	"server/service"
)

// MaintainRAGSyncTask 是阶段三的离线维护入口。
// 它只负责检查文章状态并投递异步同步任务，真正的切片和向量化由 Asynq Worker 执行。
func MaintainRAGSyncTask() (service.RAGMaintenanceResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	return service.ServiceGroupApp.RAGService.MaintainArticles(ctx)
}
