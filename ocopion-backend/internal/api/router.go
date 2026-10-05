package router

import (
	"fmt"
	"net/http"
	"runtime"

	"ocopion/internal/config"
	"ocopion/internal/module/app"
	"ocopion/internal/module/post"
	"ocopion/internal/pkg/db"

	"github.com/gin-gonic/gin"
)

func SetupRouter() (*gin.Engine, error) {
	gormDB, err := db.NewGormDB(config.GetMySQLDSN())
	if err != nil {
		return nil, err
	}
	postController := post.NewController(post.NewService(post.NewGormRepository(gormDB)))
	appController := app.NewController()
	return newRouter(postController, appController), nil
}

func newRouter(postController *post.Controller, appController *app.Controller) *gin.Engine {
	if config.IsProduction() {
		gin.SetMode(gin.ReleaseMode)
	}
	r := gin.Default()
	r.Static("/public", "./public")
	fmt.Printf("Go Version %v\n", runtime.Version())

	api := r.Group("/api")
	api.GET("/meta", appController.GetMeta)

	api.POST("/post", postController.Create)
	api.PUT("/post/:id", postController.Update)
	api.GET("/post/:id", postController.Get)
	api.GET("/post", postController.List)
	api.POST("/post/delete", postController.Delete)

	r.NoRoute(func(c *gin.Context) {
		c.JSON(http.StatusNotFound, gin.H{"msg": "404"})
	})
	return r
}
