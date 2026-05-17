package flag

import (
	"context"
	"server/service"
)

// RAGIndex 是一次性命令：go run main.go -rag-index
// 它只负责创建 rag_chunks 索引和 mapping，不会读取文章，也不会调用 Embedding API。
func RAGIndex() error {
	return service.ServiceGroupApp.RAGService.EnsureChunkIndex(context.TODO())
}

// RAGIngest 是一次性命令：go run main.go -rag-ingest
// 它会分页读取现有 article_index 中的文章，切片、向量化，然后写入 rag_chunks。
// 这个命令会调用千问 Embedding API，会产生 token/API 调用成本。
func RAGIngest() error {
	return service.ServiceGroupApp.RAGService.IngestAllArticles(context.TODO())
}
