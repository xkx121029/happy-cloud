package main

import (
	"log"

	"github.com/gin-gonic/gin"

	"happy-cloud/backend/config"
	"happy-cloud/backend/internal/db"
	"happy-cloud/backend/internal/handler"
	"happy-cloud/backend/internal/middleware"
	"happy-cloud/backend/internal/util"
)

func main() {
	config.Load()
	if err := db.Init(); err != nil {
		log.Fatalf("数据库初始化失败: %v", err)
	}
	if err := util.EnsureDir(config.Cfg.StoragePath); err != nil {
		log.Fatalf("存储目录创建失败: %v", err)
	}
	handler.InitAdmin()

	gin.SetMode(gin.ReleaseMode)
	r := gin.Default()
	r.Use(middleware.CORS())

	api := r.Group("/api")
	{
		auth := api.Group("/auth")
		auth.POST("/register", handler.Register)
		auth.POST("/login", handler.Login)
		auth.GET("/me", middleware.Auth(), handler.Me)
		auth.POST("/change-password", middleware.Auth(), handler.ChangePassword)

		user := api.Group("/user", middleware.Auth())
		user.GET("/settings", handler.GetSettings)
		user.PUT("/settings", handler.SaveSettings)

		files := api.Group("/files", middleware.Auth())
		files.GET("/list", handler.ListFiles)
		files.POST("/mkdir", handler.Mkdir)
		files.POST("/upload/hash", handler.UploadHash)
		files.POST("/upload", handler.Upload)
		files.POST("/upload/chunk", handler.UploadChunk)
		files.POST("/upload/merge", handler.UploadMerge)
		files.GET("/download", handler.Download)
		files.POST("/rename", handler.Rename)
		files.POST("/move", handler.Move)
		files.POST("/delete", handler.Delete)
		files.GET("/quota", handler.Quota)
		files.POST("/shared/toggle", handler.ToggleShared)
		files.GET("/shared/list", handler.ListShared)
		files.GET("/shared/download", handler.DownloadShared)

		transfer := api.Group("/transfer", middleware.Auth())
		transfer.GET("/users", handler.SearchUsers)
		transfer.POST("/send", handler.SendTransfer)
		transfer.GET("/incoming", handler.IncomingTransfers)
		transfer.POST("/:id/accept", handler.AcceptTransfer)
		transfer.POST("/:id/reject", handler.RejectTransfer)

		notify := api.Group("/notifications", middleware.Auth())
		notify.GET("/list", handler.ListNotifications)
		notify.POST("/:id/read", handler.ReadNotification)
		notify.GET("/unread", handler.UnreadNotifications)

		share := api.Group("/share")
		share.POST("/create", middleware.Auth(), handler.CreateShare)
		share.GET("/:token", handler.GetShare)
		share.POST("/:token/verify", handler.VerifyShare)
		share.GET("/:token/download", handler.DownloadShare)

		admin := api.Group("/admin", middleware.Auth(), middleware.Admin())
		admin.GET("/users", handler.ListUsers)
		admin.PATCH("/users/:id", handler.UpdateUser)
		admin.DELETE("/users/:id", handler.DeleteUser)
		admin.GET("/files", handler.AdminListFiles)
		admin.DELETE("/files/:id", handler.AdminDeleteFile)
		admin.GET("/stats", handler.Stats)
		admin.GET("/logs", handler.ListLogs)
	}

	log.Printf("Happy-Cloud backend 启动于 :%s", config.Cfg.ServerPort)
	if err := r.Run(":" + config.Cfg.ServerPort); err != nil {
		log.Fatal(err)
	}
}
