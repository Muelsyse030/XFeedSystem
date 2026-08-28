package routers

import (
	"XFeedSystem/internal/cache"
	"XFeedSystem/internal/handler"
	"XFeedSystem/internal/middleware"
	"XFeedSystem/internal/outbox"
	"XFeedSystem/internal/pkg/config"
	"XFeedSystem/internal/pkg/logger"
	"XFeedSystem/internal/repo"
	"XFeedSystem/internal/service"
	"context"
	"net/http"
	"net/http/pprof"
	"os"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func SetupRouter(db *gorm.DB, appCfg config.Config) *gin.Engine {

	// 生产环境不启用 gin 自带的每请求日志（与 LoggerMiddleware 重复，开销大）
	r := gin.New()
	r.Use(gin.Recovery())
	if os.Getenv("ENABLE_PPROF") == "1" {
		registerPprof(r)
	}
	r.Use(middleware.LoggerMiddleware())
	redisCache := cache.NewRedisCache(appCfg.Redis.Addr, appCfg.Redis.Password, appCfg.Redis.DB)
	searchRepo := repo.NewSearchRepo(appCfg.Meilisearch.Host, appCfg.Meilisearch.APIKey, appCfg.Meilisearch.Index)
	if err := searchRepo.EnsureIndex(context.Background()); err != nil {
		logger.Sugar.Warnf("warn: init meilisearch index: %v", err)
	}

	jwtService := middleware.NewJWT(&appCfg)

	outboxRepo := outbox.NewRepo(db)
	userRepo := repo.NewGormUserRepo(db, outboxRepo)

	notifRepo := repo.NewGormNotificationRepo(db)
	notifService := service.NewNotificationService(notifRepo, userRepo, redisCache)
	notifHandler := handler.NewNotificationHandler(notifService)

	blockRepo := repo.NewGormBlockRepo(db)
	blockService := service.NewBlockService(blockRepo, userRepo, redisCache)
	blockHandler := handler.NewBlockHandler(blockService)

	topicRepo := repo.NewGormTopicRepo(db)
	topicService := service.NewTopicService(topicRepo, redisCache)

	adminRepo := repo.NewGormAdminRepo(db)
	adminService := service.NewAdminService(adminRepo)
	adminHandler := handler.NewAdminHandler(adminService)

	userService := service.NewUserService(userRepo, redisCache, blockService, outboxRepo)
	userHandler := handler.NewUserHandler(userService, jwtService, redisCache)

	statsRepo := repo.NewGormStatsRepo(db)
	statsService := service.NewStatsService(statsRepo, redisCache)

	feedRepo := repo.NewGormFeedRepo(db)
	feedService := service.NewFeedService(feedRepo, userRepo, redisCache, searchRepo, blockService, statsService)
	feedHandler := handler.NewFeedHandler(feedService, redisCache)

	noteRepo := repo.NewGormNoteRepo(db, outboxRepo)
	noteService := service.NewNoteService(noteRepo, redisCache, blockService, topicService, userRepo, outboxRepo)
	noteHandler := handler.NewNoteHandler(noteService, userRepo, redisCache, statsService)

	topicHandler := handler.NewTopicHandler(topicService, feedService, redisCache)

	storageService, err := service.NewStorageService(appCfg)

	messageService := service.NewMessageService(repo.NewGormMessageRepo(db), userRepo, blockService)
	messageHandler := handler.NewMessageHandler(messageService)

	reportService := service.NewReportService(repo.NewGormReportRepo(db), repo.NewGormNoteRepo(db, outboxRepo), userRepo, repo.NewGormMessageRepo(db), repo.NewGormAdminRepo(db))
	reportHandler := handler.NewReportHandler(reportService)

	if err != nil {
		logger.Sugar.Warnf("warn: init oss storage: %v", err)
		storageService = &service.StorageService{} // 降级为空服务，避免 nil panic
	}
	uploadHandler := handler.NewUploadHandler(storageService)

	r.GET("/ping", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"message": "pong",
		})
	})

	// 健康检查
	r.GET("/health/live", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "alive"})
	})
	r.GET("/health/ready", func(c *gin.Context) {
		sqlDB, err := db.DB()
		if err != nil || sqlDB.Ping() != nil {
			c.JSON(503, gin.H{"status": "not ready", "reason": "database unreachable"})
			return
		}
		if err := redisCache.Ping(c.Request.Context()); err != nil {
			c.JSON(503, gin.H{"status": "not ready", "reason": "redis unreachable"})
			return
		}
		if !searchRepo.IsHealthy(c.Request.Context()) {
			c.JSON(503, gin.H{"status": "not ready", "reason": "meilisearch unreachable"})
			return
		}
		c.JSON(200, gin.H{"status": "ready"})
	})

	r.POST("/register", userHandler.Register)
	r.POST("/login", userHandler.Login)

	r.GET("/notes/:id", jwtService.OptionalJWTAuth(), noteHandler.Detail)
	r.GET("/users/:id/notes", noteHandler.ListByUser)
	r.GET("/feed", jwtService.OptionalJWTAuth(), feedHandler.List)
	r.GET("/topics/hot", topicHandler.Hot)
	r.GET("/topics/:id/feed", jwtService.OptionalJWTAuth(), topicHandler.Feed)
	r.GET("/topics/suggest", topicHandler.Suggest)
	r.GET("/users/:id", userHandler.GetProfile)
	r.GET("/search", feedHandler.Search)
	r.GET("/users/:id/following", jwtService.OptionalJWTAuth(), userHandler.ListFollowing)
	r.GET("/users/:id/followers", jwtService.OptionalJWTAuth(), userHandler.ListFollowers)

	auth := r.Group("/")
	auth.Use(jwtService.JWTAuth())
	{
		auth.GET("/me", userHandler.Me)
		auth.PATCH("/me/updata", userHandler.Updata)
		auth.GET("/users/search", userHandler.Search)
		auth.POST("/notes", noteHandler.Create)
		auth.DELETE("/notes/:id", noteHandler.Delete)
		auth.PATCH("/notes/updata/:id", noteHandler.Updata)

		auth.POST("/notes/:id/like", noteHandler.Like)
		auth.DELETE("/notes/:id/unlike", noteHandler.Unlike)
		auth.POST("/notes/:id/favorite", noteHandler.Favorite)
		auth.DELETE("/notes/:id/unfavorite", noteHandler.Unfavorite)
		auth.GET("/me/favorites", noteHandler.ListFavorites)

		auth.POST("/notes/:id/comments", noteHandler.Comment)
		auth.GET("/notes/:id/comments", noteHandler.ListComments)
		auth.DELETE("/notes/:id/comments/:comment_id", noteHandler.DeleteComment)

		auth.POST("/users/:id/follow", userHandler.Follow)
		auth.DELETE("/users/:id/unfollow", userHandler.Unfollow)
		auth.POST("/users/:id/isfollow", userHandler.Isfollow)

		auth.GET("/me/notifications", notifHandler.List)
		auth.GET("/me/notifications/unread-count", notifHandler.UnreadCount)
		auth.PATCH("/me/notifications/:id/read", notifHandler.MarkRead)
		auth.PATCH("/me/notifications/read-all", notifHandler.MarkAllRead)

		auth.POST("/users/:id/block", blockHandler.Block)
		auth.DELETE("/users/:id/unblock", blockHandler.Unblock)

		auth.POST("/upload/image", uploadHandler.Image)
		auth.POST("/upload/video", uploadHandler.Video)

		auth.POST("/messages", messageHandler.Send)
		auth.GET("/conversations", messageHandler.Conversations)
		auth.GET("/messages", messageHandler.ListWithPeer)    // ?peer_id=&cursor=&limit=
		auth.PATCH("/messages/read", messageHandler.MarkRead) // {peer_id}
		auth.GET("/messages/unread-count", messageHandler.UnreadCount)
		auth.DELETE("/messages/:id", messageHandler.Delete)

		auth.POST("/reports", reportHandler.Create)

		auth.POST("/feed/hide", feedHandler.Hide)               // {note_id}
		auth.DELETE("/feed/hide/:noteId", feedHandler.UndoHide) // 撤销

		auth.GET("/notes/:id/versions", noteHandler.ListVersions)
		auth.GET("/notes/:id/versions/:vid", noteHandler.GetVersion)
		auth.POST("/notes/:id/restore/:vid", noteHandler.RestoreVersion)

		auth.POST("/topics/:id/follow", topicHandler.Follow)
		auth.DELETE("/topics/:id/follow", topicHandler.Unfollow)
		auth.GET("/me/topics", topicHandler.Followed)
	}

	admin := auth.Group("/admin")
	admin.Use(middleware.AdminAuth())
	{
		admin.GET("/users", adminHandler.ListUsers)               // 用户列表
		admin.PATCH("/users/:id/ban", adminHandler.BanUser)       // 封禁/解封
		admin.DELETE("/notes/:id", adminHandler.DeleteNote)       // 删除笔记
		admin.DELETE("/comments/:id", adminHandler.DeleteComment) // 删除评论
		admin.GET("/stats", adminHandler.Stats)

		admin.GET("/reports", reportHandler.List)         // 待处理队列
		admin.PATCH("/reports/:id", reportHandler.Handle) // 处理// 系统统计
	}
	super := auth.Group("/admin")
	super.Use(middleware.SuperAdminAuth())
	{
		super.DELETE("/users/:id", adminHandler.DeleteUser) // 删除用户
	}
	return r
}

func registerPprof(r *gin.Engine) {
	r.GET("/debug/pprof/", gin.WrapH(http.HandlerFunc(pprof.Index)))
	r.GET("/debug/pprof/cmdline", gin.WrapH(http.HandlerFunc(pprof.Cmdline)))
	r.GET("/debug/pprof/profile", gin.WrapH(http.HandlerFunc(pprof.Profile)))
	r.GET("/debug/pprof/symbol", gin.WrapH(http.HandlerFunc(pprof.Symbol)))
	r.GET("/debug/pprof/trace", gin.WrapH(http.HandlerFunc(pprof.Trace)))
	r.GET("/debug/pprof/allocs", gin.WrapH(pprof.Handler("allocs")))
	r.GET("/debug/pprof/block", gin.WrapH(pprof.Handler("block")))
	r.GET("/debug/pprof/goroutine", gin.WrapH(pprof.Handler("goroutine")))
	r.GET("/debug/pprof/heap", gin.WrapH(pprof.Handler("heap")))
	r.GET("/debug/pprof/mutex", gin.WrapH(pprof.Handler("mutex")))
}
