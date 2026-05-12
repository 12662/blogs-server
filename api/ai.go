package api

import (
	"server/global"
	"server/model/request"
	"server/model/response"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

type AIApi struct {
}

func (aiApi *AIApi) Chat(c *gin.Context) {
	var req request.AIChat
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMessage(err.Error(), c)
		return
	}

	res, err := aiService.Chat(req)
	if err != nil {
		global.Log.Error("Failed to chat with AI:", zap.Error(err))
		response.FailWithMessage(err.Error(), c)
		return
	}
	response.OkWithData(res, c)
}
