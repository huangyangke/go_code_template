package schema

// Pagination 列表接口通用分页参数，绑定 query string.
// 复用到任意 List 接口，避免在每个 handler 重复 page/size 校验.
type Pagination struct {
	Page int `form:"page,default=1"`
	Size int `form:"size,default=20"`
}

// Normalize 将越界的分页参数收敛到合理范围（page≥1, 1≤size≤100）.
func (p *Pagination) Normalize() {
	if p.Page < 1 {
		p.Page = 1
	}
	if p.Size < 1 || p.Size > 100 {
		p.Size = 20
	}
}

// Offset 返回 SQL 查询偏移量.
func (p Pagination) Offset() int {
	return (p.Page - 1) * p.Size
}

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
