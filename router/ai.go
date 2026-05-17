package router

import (
	"server/api"

	"github.com/gin-gonic/gin"
)

type AIRouter struct {
}

func (a *AIRouter) InitAIRouter(PublicRouter *gin.RouterGroup) {
	aiRouter := PublicRouter.Group("ai")
	aiApi := api.ApiGroupApp.AIApi
	{
		aiRouter.POST("chat", aiApi.Chat)
		aiRouter.POST("chat/stream", aiApi.ChatStream)
		PublicRouter.POST("chat", aiApi.Chat)
		PublicRouter.POST("chat/stream", aiApi.ChatStream)
	}
}
