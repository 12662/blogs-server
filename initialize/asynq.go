package initialize

import (
	"context"
	"server/global"
	"server/tasks"
	"server/worker"

	"github.com/hibiken/asynq"
	"go.uber.org/zap"
)

func asynqRedisOpt() asynq.RedisClientOpt {
	redisCfg := global.Config.Redis
	return asynq.RedisClientOpt{
		Addr:     redisCfg.Address,
		Password: redisCfg.Password,
		DB:       redisCfg.DB,
	}
}

// ConnectAsynqClient 初始化任务投递客户端。
// 业务接口只通过 Client 把任务写入 Redis 队列，不直接执行耗时的向量化逻辑。
func ConnectAsynqClient() *asynq.Client {
	return asynq.NewClient(asynqRedisOpt())
}

// RunAsynqServer 启动后台 Worker。
// 这里用 goroutine 启动，避免阻塞 Gin 主服务启动；退出时 main.go 会调用 Shutdown。
func RunAsynqServer() *asynq.Server {
	server := asynq.NewServer(
		asynqRedisOpt(),
		asynq.Config{
			Concurrency: 2,
			Queues: map[string]int{
				tasks.QueueRAG: 10,
				"default":      1,
			},
			ErrorHandler: asynq.ErrorHandlerFunc(func(ctx context.Context, task *asynq.Task, err error) {
				global.Log.Error("Asynq task failed", zap.String("type", task.Type()), zap.Error(err))
			}),
		},
	)

	mux := asynq.NewServeMux()
	mux.HandleFunc(tasks.TypeSyncRAG, worker.HandleSyncRAGTask)

	go func() {
		if err := server.Run(mux); err != nil {
			global.Log.Error("Asynq server stopped", zap.Error(err))
		}
	}()

	return server
}
