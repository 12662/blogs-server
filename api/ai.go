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

func (aiApi *AIApi) ChatStream(c *gin.Context) {
	var req request.AIChat
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMessage(err.Error(), c)
		return
	}

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")

	writeEvent := func(event string, data any) error {
		c.SSEvent(event, data)
		c.Writer.Flush()
		return nil
	}

	err := aiService.ChatStream(
		c.Request.Context(),
		req,
		func(sources []response.AIArticle) error {
			return writeEvent("sources", sources)
		},
		func(delta string) error {
			return writeEvent("delta", gin.H{"content": delta})
		},
	)
	if err != nil {
		global.Log.Error("Failed to stream AI chat:", zap.Error(err))
		_ = writeEvent("error", gin.H{"message": err.Error()})
		return
	}
	_ = writeEvent("done", gin.H{"ok": true})
}
