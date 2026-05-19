package response

type AIChat struct {
	Answer  string      `json:"answer"`
	Sources []AIArticle `json:"sources"`
	Model   string      `json:"model"`
}

type AIChatModels struct {
	CurrentModel   string        `json:"current_model"`
	EmbeddingModel string        `json:"embedding_model"`
	Models         []AIChatModel `json:"models"`
}

type AIModelList struct {
	CurrentModel   string        `json:"current_model"`
	EmbeddingModel string        `json:"embedding_model"`
	Models         []AIChatModel `json:"models"`
	Total          int           `json:"total"`
}

type AIBulkCreate struct {
	Created int           `json:"created"`
	Skipped []string      `json:"skipped"`
	Models  []AIChatModel `json:"models"`
}

type AIChatModel struct {
	Name       string `json:"name"`
	ExpireAt   string `json:"expire_at"`
	Current    bool   `json:"current"`
	ShowInChat bool   `json:"show_in_chat"`
}

type AIArticle struct {
	ID       string   `json:"id"`
	Title    string   `json:"title"`
	URL      string   `json:"url"`
	Abstract string   `json:"abstract"`
	Category string   `json:"category"`
	Tags     []string `json:"tags"`
}
