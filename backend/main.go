package main

import (
	"log"

	"github.com/gin-gonic/gin"

	"happy-cloud/backend/config"
	"happy-cloud/backend/internal/db"
	"happy-cloud/backend/internal/handler"
	"happy-cloud/backend/internal/middleware"
	"happy-cloud/backend/internal/p2p"
	"happy-cloud/backend/internal/settings"
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
	// 初始化后加载系统设置缓存
	settings.Reload()

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
		// 公开站点信息（无需登录）：供前端分享链接等使用
		api.GET("/site-info", handler.SiteInfo)

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
		files.POST("/batch-move", handler.BatchMove)
		files.POST("/delete", handler.Delete)
		files.GET("/trash", handler.ListTrash)
		files.POST("/restore", handler.Restore)
		files.POST("/delete/permanent", handler.DeletePermanent)
		files.POST("/trash/clear", handler.ClearTrash)
		files.GET("/search", handler.SearchFiles)
		files.GET("/recent", handler.RecentFiles)
		files.GET("/ancestors", handler.FileAncestors)
		files.GET("/preview", handler.Preview)
		files.POST("/favorite", handler.FavoriteFile)
		files.POST("/unfavorite", handler.UnfavoriteFile)
		files.GET("/favorites", handler.ListFavorites)
		files.GET("/quota", handler.Quota)
		files.GET("/overview", handler.StorageOverview)
		files.POST("/shared/toggle", handler.ToggleShared)
		files.GET("/shared/list", handler.ListShared)
		files.GET("/shared/download", handler.DownloadShared)
		files.GET("/detail", handler.FileDetail)
		files.GET("/upload/status", handler.UploadStatus)
		files.GET("/download/batch", handler.BatchDownload)
		files.GET("/download/files", handler.ListDownloadableFiles)

		tags := api.Group("/tags", middleware.Auth())
		tags.GET("/list", handler.ListTags)
		tags.POST("/create", handler.CreateTag)
		tags.DELETE("/:id", handler.DeleteTag)
		tags.POST("/file/:id", handler.GetFileTags)
		tags.POST("/file/set", handler.SetFileTags)
		tags.GET("/search", handler.SearchTags)

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
		share.POST("/create", middleware.Auth(), handler.Share)
		share.GET("/:token", handler.GetShare)
		share.GET("/:token/download", handler.DownloadShare)
		share.GET("/mine/list", middleware.Auth(), handler.ListMyShares)
		// B6 修复：路由参数与 handler DeleteShare 读取的 c.Param("token") 统一为 :token
		// 原逻辑：share.DELETE("/mine/:id", ...) —— 参数名不匹配恒为空串导致删除恒 404
		share.DELETE("/mine/:token", middleware.Auth(), handler.DeleteShare)

		admin := api.Group("/admin", middleware.Auth(), middleware.Admin())
		admin.GET("/users", handler.ListUsers)
		admin.PATCH("/users/:id", handler.UpdateUser)
		admin.DELETE("/users/:id", handler.DeleteUser)
		admin.GET("/files", handler.AdminListFiles)
		admin.DELETE("/files/:id", handler.AdminDeleteFile)
		admin.GET("/stats", handler.Stats)
		admin.GET("/settings", handler.GetAdminSettings)
		admin.PUT("/settings", handler.SaveAdminSettings)
		admin.GET("/trends", handler.Trends)
		admin.GET("/logs", handler.ListLogs)
		admin.GET("/storage/index", handler.AdminStorageIndex)
		admin.GET("/blobs", handler.AdminListBlobs)
		admin.GET("/blobs/:hash/refs", handler.AdminBlobRefs)
		admin.POST("/storage/verify", handler.AdminStorageVerify)
	}

	// P2P 打洞配置下发（客户端读取 ws/ICE 后发起 WebRTC 协商）
	p2pSvc := p2p.NewService(r)
	p2p := api.Group("/p2p")
	p2p.GET("/config", middleware.Auth(), p2pSvc.Config)

	// 运行时 API 清单：机器可读，供各端客户端动态发现所有已注册接口
	// L7 修复：要求登录后才能查看接口清单，避免未授权暴露全部接口信息
	r.GET("/api/routes", middleware.Auth(), func(c *gin.Context) {
		rs := r.Routes()
		items := make([]gin.H, 0, len(rs))
		for _, rt := range rs {
			if len(rt.Path) >= 5 && rt.Path[:5] == "/api/" {
				items = append(items, gin.H{"method": rt.Method, "path": rt.Path})
			}
		}
		util.OK(c, items)
	})

	// 启动内置 STUN 与 host 信令协商（受 P2P_ENABLED 控制）
	p2pSvc.Start()

	log.Printf("Happy-Cloud backend 启动于 :%s", config.Cfg.ServerPort)
	if err := r.Run(":" + config.Cfg.ServerPort); err != nil {
		log.Fatal(err)
	}
}
