package config

type AI struct {
	Enable                  bool    `json:"enable" yaml:"enable"`
	QwenAPIKey              string  `json:"qwen_api_key" yaml:"qwen_api_key"`
	QwenBaseURL             string  `json:"qwen_base_url" yaml:"qwen_base_url"`
	QwenModel               string  `json:"qwen_model" yaml:"qwen_model"`
	QwenEmbeddingModel      string  `json:"qwen_embedding_model" yaml:"qwen_embedding_model"`
	EmbeddingDimensions     int     `json:"embedding_dimensions" yaml:"embedding_dimensions"`
	RAGIndex                string  `json:"rag_index" yaml:"rag_index"`
	ArticleBaseURL          string  `json:"article_base_url" yaml:"article_base_url"`
	Temperature             float64 `json:"temperature" yaml:"temperature"`
	MaxTokens               int     `json:"max_tokens" yaml:"max_tokens"`
	TopK                    int     `json:"top_k" yaml:"top_k"`
	RequestTimeout          int     `json:"request_timeout" yaml:"request_timeout"`
	EmbeddingBatchSize      int     `json:"embedding_batch_size" yaml:"embedding_batch_size"`
	EmbeddingConcurrency    int     `json:"embedding_concurrency" yaml:"embedding_concurrency"`
	RAGMaintenanceEnable    bool    `json:"rag_maintenance_enable" yaml:"rag_maintenance_enable"`
	RAGMaintenanceSpec      string  `json:"rag_maintenance_spec" yaml:"rag_maintenance_spec"`
	RAGMaintenanceBatchSize int     `json:"rag_maintenance_batch_size" yaml:"rag_maintenance_batch_size"`
}
