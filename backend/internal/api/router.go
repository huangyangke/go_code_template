package api

import (
	"github.com/gin-gonic/gin"

	"github.com/huangyangke/go_code_template/backend/internal/dao"
	"github.com/huangyangke/go_code_template/backend/internal/service"
	dbmysql "github.com/huangyangke/go-aikit/database/mysql"
	dbredis "github.com/huangyangke/go-aikit/database/redis"
)

// RegisterRoutes wires up dependencies and registers all API routes.
func RegisterRoutes(e *gin.Engine, db *dbmysql.Database, rdb *dbredis.Redis) {
	articleSvc := service.NewArticleService(dao.NewArticleDAO(db))

	v1 := e.Group("/v1")
	{
		articles := v1.Group("/articles")
		articleH := NewArticleHandler(articleSvc)
		articles.GET("", articleH.List)
		articles.POST("", articleH.Create)
		articles.GET("/:id", articleH.Get)
		articles.PUT("/:id", articleH.Update)
		articles.DELETE("/:id", articleH.Delete)
	}
}
