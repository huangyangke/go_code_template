package api

import (
	"errors"
	"strconv"

	"github.com/gin-gonic/gin"

	apperrors "github.com/example/go-template/internal/errors"
	"github.com/example/go-template/internal/schema"
	"github.com/example/go-template/internal/service"
	"github.com/example/go-template/pkg/aikit/app/response"
	"github.com/example/go-template/pkg/aikit/log"
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
		log.Error("list articles: %v", err)
	}
	response.JSONErr(c, gin.H{"total": total, "list": articles}, err)
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
		response.ParamError(c)
		return
	}

	article, err := h.svc.Create(c.Request.Context(), &req)
	if err != nil {
		log.Error("create article: %v", err)
	}
	response.JSONErr(c, article, err)
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
		response.BadRequest(c)
		return
	}

	article, err := h.svc.Get(c.Request.Context(), uint(id))
	if err != nil && !errors.As(err, new(*apperrors.AppError)) {
		log.Error("get article %d: %v", id, err)
	}
	response.JSONErr(c, article, err)
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
		response.BadRequest(c)
		return
	}

	var req schema.UpdateArticleReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.ParamError(c)
		return
	}

	article, err := h.svc.Update(c.Request.Context(), uint(id), &req)
	if err != nil && !errors.As(err, new(*apperrors.AppError)) {
		log.Error("update article %d: %v", id, err)
	}
	response.JSONErr(c, article, err)
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
		response.BadRequest(c)
		return
	}

	err = h.svc.Delete(c.Request.Context(), uint(id))
	if err != nil && !errors.As(err, new(*apperrors.AppError)) {
		log.Error("delete article %d: %v", id, err)
	}
	response.JSONErr(c, nil, err)
}
