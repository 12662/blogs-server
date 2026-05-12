package request

type AIChat struct {
	Message  string          `json:"message" binding:"required,max=2000"`
	Language string          `json:"language"`
	History  []AIChatMessage `json:"history"`
}

type AIChatMessage struct {
	Role    string `json:"role" binding:"required"`
	Content string `json:"content" binding:"required,max=4000"`
}
