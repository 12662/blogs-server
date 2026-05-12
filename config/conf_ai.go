package config

type AI struct {
	Enable         bool    `json:"enable" yaml:"enable"`
	QwenAPIKey     string  `json:"qwen_api_key" yaml:"qwen_api_key"`
	QwenBaseURL    string  `json:"qwen_base_url" yaml:"qwen_base_url"`
	QwenModel      string  `json:"qwen_model" yaml:"qwen_model"`
	Temperature    float64 `json:"temperature" yaml:"temperature"`
	MaxTokens      int     `json:"max_tokens" yaml:"max_tokens"`
	TopK           int     `json:"top_k" yaml:"top_k"`
	RequestTimeout int     `json:"request_timeout" yaml:"request_timeout"`
}
