package router

import (
	"server/api"

	"github.com/gin-gonic/gin"
)

type AIRouter struct {
}

func (a *AIRouter) InitAIRouter(PublicRouter *gin.RouterGroup, AdminRouter *gin.RouterGroup) {
	aiRouter := PublicRouter.Group("ai")
	adminAIRouter := AdminRouter.Group("ai")
	aiApi := api.ApiGroupApp.AIApi
	{
		aiRouter.POST("chat", aiApi.Chat)
		aiRouter.POST("chat/stream", aiApi.ChatStream)
		aiRouter.GET("quick-replies", aiApi.QuickReplies)
		aiRouter.PUT("current-model", aiApi.PublicCurrentModelUpdate)
		PublicRouter.POST("chat", aiApi.Chat)
		PublicRouter.POST("chat/stream", aiApi.ChatStream)
		PublicRouter.GET("chat/quick-replies", aiApi.QuickReplies)
		PublicRouter.GET("chat/models", aiApi.ChatModels)
	}
	{
		adminAIRouter.GET("models", aiApi.ModelList)
		adminAIRouter.POST("models", aiApi.ModelCreate)
		adminAIRouter.POST("models/bulk", aiApi.ModelBulkCreate)
		adminAIRouter.PUT("models", aiApi.ModelUpdate)
		adminAIRouter.DELETE("models", aiApi.ModelDelete)
		adminAIRouter.PUT("models/current", aiApi.CurrentModelUpdate)
	}
}
