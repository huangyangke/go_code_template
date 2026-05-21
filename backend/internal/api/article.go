package api

import (
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/example/go-template/pkg/aikit/app/response"

	"github.com/example/go-template/internal/schema"
	"github.com/example/go-template/internal/service"
)

// ArticleHandler handles HTTP requests for articles.
type ArticleHandler struct {
	svc *service.ArticleService
}

func NewArticleHandler(svc *service.ArticleService) *ArticleHandler {
	return &ArticleHandler{svc: svc}
}

// List godoc
// @Summary     List articles
// @Tags        articles
// @Produce     json
// @Param       page   query int false "Page number" default(1)
// @Param       size   query int false "Page size"   default(20)
// @Success     200    {object} response.ApiResponse
// @Router      /v1/articles [get]
func (h *ArticleHandler) List(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(c.DefaultQuery("size", "20"))
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 100 {
		size = 20
	}

	articles, total, err := h.svc.List(c.Request.Context(), page, size)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.JSON(c, gin.H{
		"total": total,
		"list":  articles,
	}, "")
}

// Create godoc
// @Summary     Create article
// @Tags        articles
// @Accept      json
// @Produce     json
// @Param       body body schema.CreateArticleReq true "Article"
// @Success     200  {object} response.ApiResponse
// @Router      /v1/articles [post]
func (h *ArticleHandler) Create(c *gin.Context) {
	var req schema.CreateArticleReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.ParamError(c, err.Error())
		return
	}

	article, err := h.svc.Create(c.Request.Context(), &req)
	if err != nil {
		response.InternalError(c, err.Error())
		return
	}

	response.JSON(c, article, "")
}

// Get godoc
// @Summary     Get article by ID
// @Tags        articles
// @Produce     json
// @Param       id  path int true "Article ID"
// @Success     200 {object} response.ApiResponse
// @Router      /v1/articles/{id} [get]
func (h *ArticleHandler) Get(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "invalid id")
		return
	}

	article, err := h.svc.Get(c.Request.Context(), uint(id))
	if err != nil {
		response.JSONErr(c, nil, err)
		return
	}

	response.JSON(c, article, "")
}

// Update godoc
// @Summary     Update article
// @Tags        articles
// @Accept      json
// @Produce     json
// @Param       id   path int                    true "Article ID"
// @Param       body body schema.UpdateArticleReq true "Article"
// @Success     200  {object} response.ApiResponse
// @Router      /v1/articles/{id} [put]
func (h *ArticleHandler) Update(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "invalid id")
		return
	}

	var req schema.UpdateArticleReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.ParamError(c, err.Error())
		return
	}

	article, err := h.svc.Update(c.Request.Context(), uint(id), &req)
	if err != nil {
		response.JSONErr(c, nil, err)
		return
	}

	response.JSON(c, article, "")
}

// Delete godoc
// @Summary     Delete article
// @Tags        articles
// @Produce     json
// @Param       id path int true "Article ID"
// @Success     200 {object} response.ApiResponse
// @Router      /v1/articles/{id} [delete]
func (h *ArticleHandler) Delete(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "invalid id")
		return
	}

	if err := h.svc.Delete(c.Request.Context(), uint(id)); err != nil {
		response.JSONErr(c, nil, err)
		return
	}

	response.JSON(c, nil, "")
}
