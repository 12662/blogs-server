package api

import (
	"server/global"
	"server/model/response"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

type WechatApi struct {
}

func (wechatApi *WechatApi) JSConfig(c *gin.Context) {
	pageURL := c.Query("url")
	config, err := wechatService.JSConfig(c.Request.Context(), pageURL)
	if err != nil {
		global.Log.Error("failed to create wechat js config", zap.Error(err))
		response.FailWithMessage(err.Error(), c)
		return
	}

	response.OkWithData(config, c)
}
