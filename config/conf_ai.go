package config

import "strings"

type AI struct {
	Enable                  bool      `json:"enable" yaml:"enable"`
	QwenAPIKey              string    `json:"qwen_api_key" yaml:"qwen_api_key"`
	QwenBaseURL             string    `json:"qwen_base_url" yaml:"qwen_base_url"`
	QwenModel               string    `json:"qwen_model" yaml:"qwen_model"`
	QwenEmbeddingModel      string    `json:"qwen_embedding_model" yaml:"qwen_embedding_model"`
	EmbeddingDimensions     int       `json:"embedding_dimensions" yaml:"embedding_dimensions"`
	RAGIndex                string    `json:"rag_index" yaml:"rag_index"`
	ArticleBaseURL          string    `json:"article_base_url" yaml:"article_base_url"`
	Temperature             float64   `json:"temperature" yaml:"temperature"`
	MaxTokens               int       `json:"max_tokens" yaml:"max_tokens"`
	TopK                    int       `json:"top_k" yaml:"top_k"`
	RequestTimeout          int       `json:"request_timeout" yaml:"request_timeout"`
	EmbeddingBatchSize      int       `json:"embedding_batch_size" yaml:"embedding_batch_size"`
	EmbeddingConcurrency    int       `json:"embedding_concurrency" yaml:"embedding_concurrency"`
	RAGMaintenanceEnable    bool      `json:"rag_maintenance_enable" yaml:"rag_maintenance_enable"`
	RAGMaintenanceSpec      string    `json:"rag_maintenance_spec" yaml:"rag_maintenance_spec"`
	RAGMaintenanceBatchSize int       `json:"rag_maintenance_batch_size" yaml:"rag_maintenance_batch_size"`
	IntroReply              string    `json:"intro_reply" yaml:"intro_reply"`
	TechStackReply          string    `json:"tech_stack_reply" yaml:"tech_stack_reply"`
	ChatModels              []AIModel `json:"chat_models" yaml:"chat_models"`
}

type AIModel struct {
	Name       string `json:"name" yaml:"name"`
	ExpireAt   string `json:"expire_at" yaml:"expire_at"`
	ShowInChat bool   `json:"show_in_chat" yaml:"show_in_chat"`
}

func (ai AI) CurrentChatModel() string {
	model := strings.TrimSpace(ai.QwenModel)
	if model == "" {
		return "qwen-turbo-latest"
	}
	return model
}

func (ai AI) EffectiveChatModels() []AIModel {
	models := make([]AIModel, 0, len(ai.ChatModels)+1)
	seen := make(map[string]struct{})
	for _, model := range ai.ChatModels {
		name := strings.TrimSpace(model.Name)
		if name == "" {
			continue
		}
		model.Name = name
		model.ExpireAt = strings.TrimSpace(model.ExpireAt)
		key := model.Name + "\x00" + model.ExpireAt
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		models = append(models, model)
	}

	current := ai.CurrentChatModel()
	if current != "" {
		hasCurrent := false
		for _, model := range models {
			if model.Name == current {
				hasCurrent = true
				break
			}
		}
		if !hasCurrent {
			models = append(models, AIModel{Name: current, ShowInChat: false})
		}
	}
	return models
}

func (ai AI) VisibleChatModels() []AIModel {
	allModels := ai.EffectiveChatModels()
	models := make([]AIModel, 0, len(allModels))
	for _, model := range allModels {
		if model.ShowInChat {
			models = append(models, model)
		}
	}
	return models
}
