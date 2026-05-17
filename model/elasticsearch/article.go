package elasticsearch

import (
	"github.com/elastic/go-elasticsearch/v8/typedapi/types"
)

const (
	RAGSyncStatusPending   = "pending"
	RAGSyncStatusSuccess   = "success"
	RAGSyncStatusFailed    = "failed"
	RAGSyncStatusUntracked = "untracked"
)

// Article 文章表
type Article struct {
	CreatedAt string `json:"created_at"` // 创建时间
	UpdatedAt string `json:"updated_at"` // 更新时间

	Cover    string   `json:"cover"`    // 文章封面
	Title    string   `json:"title"`    // 文章标题
	Keyword  string   `json:"keyword"`  // 文章标题-关键字
	Category string   `json:"category"` // 文章类别
	Tags     []string `json:"tags"`     // 文章标签
	Abstract string   `json:"abstract"` // 文章简介
	Content  string   `json:"content"`  // 文章内容

	Views    int `json:"views"`    // 浏览量
	Comments int `json:"comments"` // 评论量
	Likes    int `json:"likes"`    // 收藏量

	// RAGSyncStatus 表示该文章的向量同步状态。
	// 当前项目的文章主体存储在 ES article_index 中，所以状态也先落在 ES 文档里；
	// 如果后续迁移到 MySQL articles 表，可用同名字段保持接口兼容。
	RAGSyncStatus string `json:"rag_sync_status"`
	RAGSyncError  string `json:"rag_sync_error"`
	RAGChunkCount int    `json:"rag_chunk_count"`
	// RAGContentHash 是文章标题/摘要/正文清洗后的内容指纹。
	// 定时维护任务会用它判断“文章已更新，但向量还是旧版本”的情况。
	RAGContentHash string `json:"rag_content_hash"`
	RAGSyncedAt    string `json:"rag_synced_at"`
}

// ArticleIndex 文章 ES 索引
func ArticleIndex() string {
	return "article_index"
}

// ArticleMapping 文章 Mapping 映射
func ArticleMapping() *types.TypeMapping {
	return &types.TypeMapping{
		Properties: map[string]types.Property{
			"created_at":       types.DateProperty{NullValue: nil, Format: func(s string) *string { return &s }("yyyy-MM-dd HH:mm:ss")},
			"updated_at":       types.DateProperty{NullValue: nil, Format: func(s string) *string { return &s }("yyyy-MM-dd HH:mm:ss")},
			"cover":            types.TextProperty{},
			"title":            types.TextProperty{},
			"keyword":          types.KeywordProperty{},
			"category":         types.KeywordProperty{},
			"tags":             []types.KeywordProperty{},
			"abstract":         types.TextProperty{},
			"content":          types.TextProperty{},
			"views":            types.IntegerNumberProperty{},
			"comments":         types.IntegerNumberProperty{},
			"likes":            types.IntegerNumberProperty{},
			"rag_sync_status":  types.KeywordProperty{},
			"rag_sync_error":   types.TextProperty{},
			"rag_chunk_count":  types.IntegerNumberProperty{},
			"rag_content_hash": types.KeywordProperty{},
			"rag_synced_at":    types.DateProperty{NullValue: nil, Format: func(s string) *string { return &s }("yyyy-MM-dd HH:mm:ss")},
		},
	}
}
