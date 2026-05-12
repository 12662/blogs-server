package response

type AIChat struct {
	Answer     string      `json:"answer"`
	References []AIArticle `json:"references"`
}

type AIArticle struct {
	ID       string   `json:"id"`
	Title    string   `json:"title"`
	URL      string   `json:"url"`
	Abstract string   `json:"abstract"`
	Category string   `json:"category"`
	Tags     []string `json:"tags"`
}
