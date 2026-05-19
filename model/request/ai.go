package request

type AIChat struct {
	Message  string          `json:"message" binding:"required,max=2000"`
	Language string          `json:"language"`
	Model    string          `json:"model" binding:"max=200"`
	History  []AIChatMessage `json:"history"`
}

type AIChatMessage struct {
	Role    string `json:"role" binding:"required"`
	Content string `json:"content" binding:"required,max=4000"`
}

type AIModel struct {
	Name       string `json:"name" binding:"required,max=200"`
	ExpireAt   string `json:"expire_at" binding:"max=20"`
	ShowInChat bool   `json:"show_in_chat"`
}

type AIBulkModels struct {
	Models []AIModel `json:"models" binding:"required"`
}

type AIModelUpdate struct {
	OldName     string `json:"old_name" binding:"required,max=200"`
	OldExpireAt string `json:"old_expire_at" binding:"max=20"`
	Name        string `json:"name" binding:"required,max=200"`
	ExpireAt    string `json:"expire_at" binding:"max=20"`
	ShowInChat  bool   `json:"show_in_chat"`
}

type AIModelDelete struct {
	Name     string `json:"name" binding:"required,max=200"`
	ExpireAt string `json:"expire_at" binding:"max=20"`
}

type AICurrentModel struct {
	Model string `json:"model" binding:"required,max=200"`
}
