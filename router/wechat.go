package router

import (
	"server/api"

	"github.com/gin-gonic/gin"
)

type WechatRouter struct {
}

func (w *WechatRouter) InitWechatRouter(PublicRouter *gin.RouterGroup) {
	wechatRouter := PublicRouter.Group("wechat")
	wechatApi := api.ApiGroupApp.WechatApi
	{
		wechatRouter.GET("js-config", wechatApi.JSConfig)
	}
}
