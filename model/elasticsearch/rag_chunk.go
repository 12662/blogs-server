package elasticsearch

// RagChunk 是 RAG 向量索引中的最小检索单元。
// 注意它不是一篇完整文章，而是一篇文章切分后的一个文本片段。
// 这样做的好处是：检索时可以命中更精确的上下文，避免整篇文章过长导致 Prompt 噪声过多。
type RagChunk struct {
	// ID 使用 sha256(url) 的前 12 位 + chunk 序号生成。
	// 同一篇文章、同一切片顺序会得到稳定 ID，方便更新时覆盖写入。
	ID            string    `json:"id"`
	ArticleID     string    `json:"article_id"`     // 原文章在 article_index 中的文档 ID，用于删除/更新联动。
	ContentVector []float64 `json:"content_vector"` // text-embedding-v4 输出的 1024 维向量。
	Text          string    `json:"text"`           // 纯文本切片，最终会拼接进大模型 Prompt。
	Title         string    `json:"title"`          // 原文章标题，返回给前端作为引用来源。
	URL           string    `json:"url"`            // 原文章绝对 URL，前端点击引用时使用。
	Language      string    `json:"language"`       // 当前只入库中文内容，所以默认是 zh。
}

// RagChunkIndex 返回默认向量切片索引名。
// 实际运行时优先使用 config.yaml 里的 ai.rag_index，方便以后换索引名。
func RagChunkIndex() string {
	return "rag_chunks"
}
