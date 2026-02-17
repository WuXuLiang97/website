package main

import (
	"fmt"
	"os"
	"path/filepath"

	"anime-website/config"
	"anime-website/handlers"
	"anime-website/services"

	"github.com/gin-gonic/gin"
)

const (
	hlsDir       = "static/hls"
	templatesDir = "templates"
)

func main() {
	logger, logFile := config.GetLogger()
	defer func() {
		if logFile != nil {
			logFile.Close()
		}
	}()

	config.Init()

	services.InitDB()

	services.StorageServiceInstance.Init()

	if _, err := os.Stat(hlsDir); os.IsNotExist(err) {
		logger.Printf("创建HLS目录: %s\n", hlsDir)
		err = os.MkdirAll(hlsDir, 0755)
		if err != nil {
			logger.Printf("错误: 创建HLS目录失败: %v\n", err)
		}
	}

	cfg := config.Get()
	if cfg.Log.Level == "debug" {
		gin.SetMode(gin.DebugMode)
	} else {
		gin.SetMode(gin.ReleaseMode)
	}

	r := gin.Default()

	r.LoadHTMLGlob(filepath.Join(templatesDir, "*.html"))

	r.Static("/static", "./static")
	r.Static("/hls", "./static/hls")

	authHandler := handlers.NewAuthHandler()
	playHistoryHandler := handlers.NewPlayHistoryHandler(authHandler)
	videoHandler := handlers.NewVideoHandler()

	r.GET("/", videoHandler.Index)
	r.GET("/search", videoHandler.Search)
	r.GET("/play", videoHandler.Play)
	r.GET("/history", videoHandler.History)
	r.GET("/hls", videoHandler.HLS)
	r.GET("/api/videos", videoHandler.VideoList)
	r.POST("/api/scan-videos", videoHandler.ScanVideos)
	r.POST("/api/batch-hls", videoHandler.BatchHLS)
	r.POST("/api/batch-hls/stop", videoHandler.StopBatchHLS)

	r.POST("/api/play-history/save", playHistoryHandler.SavePlayHistory)
	r.GET("/api/play-history/get", playHistoryHandler.GetPlayHistory)
	r.GET("/api/play-history/all", playHistoryHandler.GetAllPlayHistory)
	r.DELETE("/api/play-history/delete", playHistoryHandler.DeletePlayHistory)
	r.DELETE("/api/play-history/clear", playHistoryHandler.ClearAllPlayHistory)

	r.POST("/api/auth/register", authHandler.Register)
	r.POST("/api/auth/login", authHandler.Login)
	r.GET("/api/auth/user", authHandler.GetCurrentUser)
	r.POST("/api/auth/logout", authHandler.Logout)
	r.POST("/api/auth/renew", authHandler.RenewCookie)

	r.GET("/api/animes/search", videoHandler.SearchAnimes)
	r.DELETE("/api/animes/delete", videoHandler.DeleteAnime)
	r.POST("/api/hls/fix", videoHandler.FixHLSVideos)
	r.GET("/api/hls/fix", videoHandler.FixHLSVideos)
	r.GET("/hls-fix", videoHandler.HLSFix)
	r.GET("/login", videoHandler.LoginPage)
	r.GET("/register", videoHandler.RegisterPage)

	disks := cfg.Storage.Disks
	for _, disk := range disks {
		if disk.Enabled {
			r.Static("/storage/"+disk.Name, disk.Path)
			logger.Printf("添加存储路由: /storage/%s -> %s\n", disk.Name, disk.Path)
		}
	}

	port := cfg.Server.Port
	listenAddr := fmt.Sprintf("[::]:%d", port)
	primaryIPv6Addr := "240e:351:580a:6400:55ab:f75f:cee7:1da1"
	logger.Printf("服务器启动成功！监听端口: %d\n", port)
	logger.Printf("访问地址: http://localhost:%d\n", port)
	logger.Printf("HLS批量生成页面: http://localhost:%d/hls\n", port)
	logger.Printf("IPv6访问地址: http://[%s]:%d\n", primaryIPv6Addr, port)

	go func() {
		logger.Println("开始异步同步本地动画到数据库...")
		services.VideoServiceInstance.ScanVideos()
		logger.Println("异步同步本地动画到数据库完成！")
	}()

	err := r.Run(listenAddr)
	if err != nil {
		logger.Fatalf("服务器启动失败: %v\n", err)
	}
}
