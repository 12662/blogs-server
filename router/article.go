package router

import (
	"server/api"

	"github.com/gin-gonic/gin"
)

type ArticleRouter struct {
}

func (a *ArticleRouter) InitArticleRouter(Router *gin.RouterGroup, PublicRouter *gin.RouterGroup, AdminRouter *gin.RouterGroup) {
	articleRouter := Router.Group("article")
	articlePublicRouter := PublicRouter.Group("article")
	articleAdminRouter := AdminRouter.Group("article")
	articleAdminSyncRouter := AdminRouter.Group("admin/articles")

	articleApi := api.ApiGroupApp.ArticleApi
	{
		articleRouter.POST("like", articleApi.ArticleLike)
		articleRouter.GET("isLike", articleApi.ArticleIsLike)
		articleRouter.GET("likesList", articleApi.ArticleLikesList)
	}
	{
		articlePublicRouter.GET(":id", articleApi.ArticleInfoByID)
		articlePublicRouter.GET("search", articleApi.ArticleSearch)
		articlePublicRouter.GET("category", articleApi.ArticleCategory)
		articlePublicRouter.GET("tags", articleApi.ArticleTags)
	}
	{
		articleAdminRouter.POST("create", articleApi.ArticleCreate)
		articleAdminRouter.DELETE("delete", articleApi.ArticleDelete)
		articleAdminRouter.PUT("update", articleApi.ArticleUpdate)
		articleAdminRouter.GET("list", articleApi.ArticleList)
		articleAdminRouter.POST("sync/:id", articleApi.ArticleSyncRetry)
		articleAdminRouter.DELETE("sync/:id", articleApi.ArticleSyncClear)
	}
	{
		// 管理端新路径：/api/admin/articles/sync/:id。
		// 这里仍然挂在 AdminRouter 下，所以会复用 JWT + 管理员权限中间件。
		articleAdminSyncRouter.POST("sync/:id", articleApi.ArticleSyncRetry)
		articleAdminSyncRouter.DELETE("sync/:id", articleApi.ArticleSyncClear)
	}
}
