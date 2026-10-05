package main

import (
	"fmt"

	router "ocopion/internal/api"
	"ocopion/internal/config"
	"ocopion/internal/logger"
)

// @title ocopion-backend API
// @version 1.0
// @description ocopion-backend HTTP API documentation.
// @host localhost:8301
// @BasePath /api
// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization
func main() {
	// 初始化配置
	if err := config.Init(); err != nil {
		fmt.Printf("❌ init config failed, err: %v\n", err)
		return
	}

	if err := logger.Init(); err != nil {
		fmt.Printf("❌ init logger failed, err:%v\n", err)
		return
	}

	// 注册路由
	r, err := router.SetupRouter()
	if err != nil {
		fmt.Printf("❌ 初始化路由失败: %v\n", err)
		return
	}

	// 显示启动信息
	addr := config.GetServerAddr()
	fmt.Printf("\n🚀 服务正在启动...\n")
	fmt.Printf("🔗 地址: http://localhost%s\n", addr)

	// 启动HTTP服务，使用配置文件中的端口
	err = r.Run(addr)
	if err != nil {
		fmt.Printf("❌ 启动服务器失败: %v\n", err)
		return
	}
}
