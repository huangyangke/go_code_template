package schema

// CreateArticleReq is the request body for creating an article.
type CreateArticleReq struct {
	Title   string `json:"title"   binding:"required,max=255"`
	Content string `json:"content" binding:"required"`
	Author  string `json:"author"  binding:"required,max=100"`
}

// UpdateArticleReq is the request body for updating an article.
type UpdateArticleReq struct {
	Title   string `json:"title"   binding:"omitempty,max=255"`
	Content string `json:"content" binding:"omitempty"`
}
