package router

import (
	"fmt"
	"net/http"
	"runtime"

	"ocopion/internal/config"
	"ocopion/internal/module/app"
	"ocopion/internal/module/cloudflare"
	"ocopion/internal/module/post"
	"ocopion/internal/module/video"
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
	videoController := video.NewVideoController(video.NewVideoService(video.NewVideoRepository(), config.GetConfig()))
	return newRouter(postController, appController, videoController), nil
}

func newRouter(postController *post.Controller, appController *app.Controller, videoController *video.VideoController) *gin.Engine {
	if config.IsProduction() {
		gin.SetMode(gin.ReleaseMode)
	}
	r := gin.Default()
	r.Static("/public", "./public")
	fmt.Printf("Go Version %v\n", runtime.Version())

	api := r.Group("/api")
	cfController := cloudflare.NewController(cloudflare.NewService(cloudflare.NewRepository(config.GetConfig())))
	api.POST("/cloudflare/fast-ip", cfController.Start)
	api.GET("/cloudflare/fast-ip", cfController.Get)

	api.GET("/meta", appController.GetMeta)
	api.GET("/video", videoController.GetVideoList)

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
