package router

import (
	"fmt"
	"net/http"
	"runtime"

	"ocopion/internal/config"
	"ocopion/internal/middleware"
	"ocopion/internal/module/app"
	"ocopion/internal/module/auth"
	"ocopion/internal/module/cloudflare"
	"ocopion/internal/module/post"
	"ocopion/internal/module/user"
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
	userRepo := user.NewGormRepository(gormDB)
	userController := user.NewController(user.NewService(userRepo))
	authController := auth.NewAuthController(auth.NewAuthService(userRepo))
	return newRouter(postController, userController, authController, appController, videoController), nil
}

func newRouter(postController *post.Controller, userController *user.Controller, authController *auth.AuthController, appController *app.Controller, videoController *video.VideoController) *gin.Engine {
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
	api.POST("/cloudflare/speed-test", cfController.SpeedTest)

	api.GET("/meta", appController.GetMeta)
	api.GET("/video", videoController.GetVideoList)

	api.POST("/auth/login", authController.Login)
	api.POST("/auth/refresh", authController.Refresh)
	authenticate := middleware.JWTAuthMiddleware()

	postGroup := api.Group("/post")
	postGroup.Use(authenticate)
	{
		postGroup.POST("", postController.Create)
		postGroup.PUT("/:id", postController.Update)
		postGroup.GET("/:id", postController.Get)
		postGroup.GET("", postController.List)
		postGroup.POST("/delete", postController.Delete)
	}

	userGroup := api.Group("/user")
	userGroup.Use(authenticate)
	{
		userGroup.POST("", userController.Create)
		userGroup.PUT("/:id", userController.Update)
		userGroup.GET("/:id", userController.Get)
		userGroup.GET("", userController.List)
		userGroup.POST("/delete", userController.Delete)
		userGroup.GET("/me", userController.Me)
	}

	r.NoRoute(func(c *gin.Context) {
		c.JSON(http.StatusNotFound, gin.H{"msg": "404"})
	})
	return r
}
