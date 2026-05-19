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
		func(model string) error {
			return writeEvent("model", gin.H{"model": model})
		},
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

func (aiApi *AIApi) QuickReplies(c *gin.Context) {
	response.OkWithData(gin.H{
		"intro_reply":      global.Config.AI.IntroReply,
		"tech_stack_reply": global.Config.AI.TechStackReply,
	}, c)
}

func (aiApi *AIApi) ChatModels(c *gin.Context) {
	currentModel := global.Config.AI.CurrentChatModel()
	models := make([]response.AIChatModel, 0, len(global.Config.AI.VisibleChatModels()))
	for _, model := range global.Config.AI.VisibleChatModels() {
		models = append(models, response.AIChatModel{
			Name:       model.Name,
			ExpireAt:   model.ExpireAt,
			Current:    model.Name == currentModel,
			ShowInChat: model.ShowInChat,
		})
	}
	response.OkWithData(response.AIChatModels{
		CurrentModel:   currentModel,
		EmbeddingModel: global.Config.AI.QwenEmbeddingModel,
		Models:         models,
	}, c)
}

func (aiApi *AIApi) ModelList(c *gin.Context) {
	response.OkWithData(aiService.ListModels(), c)
}

func (aiApi *AIApi) ModelCreate(c *gin.Context) {
	var req request.AIModel
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMessage(err.Error(), c)
		return
	}
	if err := aiService.CreateModel(req); err != nil {
		response.FailWithMessage(err.Error(), c)
		return
	}
	response.OkWithMessage("Successfully created model", c)
}

func (aiApi *AIApi) ModelBulkCreate(c *gin.Context) {
	var req request.AIBulkModels
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMessage(err.Error(), c)
		return
	}
	res, err := aiService.BulkCreateModels(req)
	if err != nil {
		response.FailWithMessage(err.Error(), c)
		return
	}
	response.OkWithDetailed(res, "Successfully created models", c)
}

func (aiApi *AIApi) ModelUpdate(c *gin.Context) {
	var req request.AIModelUpdate
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMessage(err.Error(), c)
		return
	}
	if err := aiService.UpdateModel(req); err != nil {
		response.FailWithMessage(err.Error(), c)
		return
	}
	response.OkWithMessage("Successfully updated model", c)
}

func (aiApi *AIApi) ModelDelete(c *gin.Context) {
	var req request.AIModelDelete
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMessage(err.Error(), c)
		return
	}
	if err := aiService.DeleteModel(req); err != nil {
		response.FailWithMessage(err.Error(), c)
		return
	}
	response.OkWithMessage("Successfully deleted model", c)
}

func (aiApi *AIApi) CurrentModelUpdate(c *gin.Context) {
	var req request.AICurrentModel
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMessage(err.Error(), c)
		return
	}
	if err := aiService.SetCurrentModel(req.Model); err != nil {
		response.FailWithMessage(err.Error(), c)
		return
	}
	response.OkWithMessage("Successfully updated current model", c)
}

func (aiApi *AIApi) PublicCurrentModelUpdate(c *gin.Context) {
	var req request.AICurrentModel
	if err := c.ShouldBindJSON(&req); err != nil {
		response.FailWithMessage(err.Error(), c)
		return
	}
	if err := aiService.SetVisibleCurrentModel(req.Model); err != nil {
		response.FailWithMessage(err.Error(), c)
		return
	}
	response.OkWithMessage("Successfully updated current model", c)
}
