package api

import (
	"server/global"
	"server/model/request"
	"server/model/response"
	"server/utils"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

type ArticleApi struct {
}

// ArticleInfoByID 根据文章id获取文章内容
func (articleApi *ArticleApi) ArticleInfoByID(c *gin.Context) {
	var req request.ArticleInfoByID
	err := c.ShouldBindUri(&req)
	if err != nil {
		response.FailWithMessage(err.Error(), c)
		return
	}

	article, err := articleService.ArticleInfoByID(req.ID)
	if err != nil {
		global.Log.Error("Failed to get article information:", zap.Error(err))
		response.FailWithMessage("Failed to get article information", c)
		return
	}
	response.OkWithData(article, c)
}

// ArticleSearch 文章搜索
func (articleApi *ArticleApi) ArticleSearch(c *gin.Context) {
	var info request.ArticleSearch
	err := c.ShouldBindQuery(&info)
	if err != nil {
		response.FailWithMessage(err.Error(), c)
		return
	}

	list, total, err := articleService.ArticleSearch(info)
	if err != nil {
		global.Log.Error("Failed to get article search results:", zap.Error(err))
		response.FailWithMessage("Failed to get article search results", c)
		return
	}
	response.OkWithData(response.PageResult{
		List:  list,
		Total: total,
	}, c)
}

// ArticleCategory 获取所有文章类别及数量
func (articleApi *ArticleApi) ArticleCategory(c *gin.Context) {
	category, err := articleService.ArticleCategory()
	if err != nil {
		global.Log.Error("Failed to get article category:", zap.Error(err))
		response.FailWithMessage("Failed to get article category", c)
		return
	}
	response.OkWithData(category, c)
}

// ArticleTags 获取所有文章标签及数量
func (articleApi *ArticleApi) ArticleTags(c *gin.Context) {
	tags, err := articleService.ArticleTags()
	if err != nil {
		global.Log.Error("Failed to get article tags:", zap.Error(err))
		response.FailWithMessage("Failed to get article tags", c)
		return
	}
	response.OkWithData(tags, c)
}

// ArticleLike 文章收藏操作，收藏文章或者取消收藏
func (articleApi *ArticleApi) ArticleLike(c *gin.Context) {
	var req request.ArticleLike
	err := c.ShouldBindJSON(&req)
	if err != nil {
		response.FailWithMessage(err.Error(), c)
		return
	}

	req.UserID = utils.GetUserID(c)
	err = articleService.ArticleLike(req)
	if err != nil {
		global.Log.Error("Failed to complete the operation:", zap.Error(err))
		response.FailWithMessage("Failed to complete the operation", c)
		return
	}
	response.OkWithMessage("Successfully completed the operation", c)
}

// ArticleIsLike 返回文章收藏状态，用户是否收藏该文章
func (articleApi *ArticleApi) ArticleIsLike(c *gin.Context) {
	var req request.ArticleLike
	err := c.ShouldBindQuery(&req)
	if err != nil {
		response.FailWithMessage(err.Error(), c)
		return
	}

	req.UserID = utils.GetUserID(c)
	isLike, err := articleService.ArticleIsLike(req)
	if err != nil {
		global.Log.Error("Failed to get like status:", zap.Error(err))
		response.FailWithMessage("Failed to get like status", c)
		return
	}
	response.OkWithData(isLike, c)
}

// ArticleLikesList 获取文章收藏列表
func (articleApi *ArticleApi) ArticleLikesList(c *gin.Context) {
	var pageInfo request.ArticleLikesList
	err := c.ShouldBindQuery(&pageInfo)
	if err != nil {
		response.FailWithMessage(err.Error(), c)
		return
	}

	pageInfo.UserID = utils.GetUserID(c)
	list, total, err := articleService.ArticleLikesList(pageInfo)
	if err != nil {
		global.Log.Error("Failed to get likes list:", zap.Error(err))
		response.FailWithMessage("Failed to get likes list", c)
		return
	}
	response.OkWithData(response.PageResult{
		List:  list,
		Total: total,
	}, c)
}

// ArticleCreate 发布文章
func (articleApi *ArticleApi) ArticleCreate(c *gin.Context) {
	var req request.ArticleCreate
	err := c.ShouldBindJSON(&req)
	if err != nil {
		response.FailWithMessage(err.Error(), c)
		return
	}

	err = articleService.ArticleCreate(req)
	if err != nil {
		global.Log.Error("Failed to create article:", zap.Error(err))
		response.FailWithMessage("Failed to create article", c)
		return
	}
	response.OkWithMessage("Successfully created article", c)
}

// ArticleDelete 删除文章
func (articleApi *ArticleApi) ArticleDelete(c *gin.Context) {
	var req request.ArticleDelete
	err := c.ShouldBindJSON(&req)
	if err != nil {
		response.FailWithMessage(err.Error(), c)
		return
	}

	err = articleService.ArticleDelete(req)
	if err != nil {
		global.Log.Error("Failed to delete article:", zap.Error(err))
		response.FailWithMessage("Failed to delete article", c)
		return
	}
	response.OkWithMessage("Successfully deleted article", c)
}

// ArticleUpdate 更新文章
func (articleApi *ArticleApi) ArticleUpdate(c *gin.Context) {
	var req request.ArticleUpdate
	err := c.ShouldBindJSON(&req)
	if err != nil {
		response.FailWithMessage(err.Error(), c)
		return
	}

	err = articleService.ArticleUpdate(req)
	if err != nil {
		global.Log.Error("Failed to update article:", zap.Error(err))
		response.FailWithMessage("Failed to update article", c)
		return
	}
	response.OkWithMessage("Successfully updated article", c)
}

// ArticleSyncRetry 管理员手动重试某篇文章的 RAG 向量同步。
// 这个接口只负责把任务重新放进 Redis 队列，不能在 HTTP 请求里直接调用千问 API，
// 否则后台管理操作会被 AI 服务耗时或失败拖住。
func (articleApi *ArticleApi) ArticleSyncRetry(c *gin.Context) {
	var req request.ArticleInfoByID
	if err := c.ShouldBindUri(&req); err != nil {
		response.FailWithMessage(err.Error(), c)
		return
	}

	if err := articleService.RetryRAGSync(c.Request.Context(), req.ID); err != nil {
		global.Log.Error("Failed to enqueue rag sync retry:", zap.String("article_id", req.ID), zap.Error(err))
		response.FailWithMessage("Failed to enqueue rag sync retry", c)
		return
	}
	response.OkWithMessage("Successfully queued RAG sync", c)
}

// ArticleSyncClear 管理员手动清除某篇文章在 rag_chunks 中的向量切片。
// 注意：这里不会删除原文章索引，只会删除 AI 检索使用的切片数据，并把状态标记为 untracked。
func (articleApi *ArticleApi) ArticleSyncClear(c *gin.Context) {
	var req request.ArticleInfoByID
	if err := c.ShouldBindUri(&req); err != nil {
		response.FailWithMessage(err.Error(), c)
		return
	}

	if err := articleService.ClearRAGSync(c.Request.Context(), req.ID); err != nil {
		global.Log.Error("Failed to clear rag sync data:", zap.String("article_id", req.ID), zap.Error(err))
		response.FailWithMessage("Failed to clear rag sync data", c)
		return
	}
	response.OkWithMessage("Successfully cleared RAG sync data", c)
}

// ArticleList 获取文章列表
func (articleApi *ArticleApi) ArticleList(c *gin.Context) {
	var pageInfo request.ArticleList
	err := c.ShouldBindQuery(&pageInfo)
	if err != nil {
		response.FailWithMessage(err.Error(), c)
		return
	}

	list, total, err := articleService.ArticleList(pageInfo)
	if err != nil {
		global.Log.Error("Failed to get article list:", zap.Error(err))
		response.FailWithMessage("Failed to get article list", c)
		return
	}
	response.OkWithData(response.PageResult{
		List:  list,
		Total: total,
	}, c)
}
