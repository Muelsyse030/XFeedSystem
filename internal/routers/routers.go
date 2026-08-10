package routers

import (
	"context"

	"XFeedSystem/internal/cache"
	"XFeedSystem/internal/handler"
	"XFeedSystem/internal/middleware"
	"XFeedSystem/internal/pkg/config"
	"XFeedSystem/internal/repo"
	"XFeedSystem/internal/service"
	"XFeedSystem/internal/pkg/logger"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func SetupRouter(db *gorm.DB, appCfg config.Config) *gin.Engine {

	r := gin.Default()
	r.Use(middleware.LoggerMiddleware())
	redisCache := cache.NewRedisCache(appCfg.Redis.Addr, appCfg.Redis.Password, appCfg.Redis.DB)
	searchRepo := repo.NewSearchRepo(appCfg.Meilisearch.Host, appCfg.Meilisearch.APIKey, appCfg.Meilisearch.Index)
	if err := searchRepo.EnsureIndex(context.Background()); err != nil {
		logger.Sugar.Warnf("warn: init meilisearch index: %v", err)
	}

	jwtService := middleware.NewJWT(&appCfg)

	userRepo := repo.NewGormUserRepo(db)

	notifRepo := repo.NewGormNotificationRepo(db)
	notifService := service.NewNotificationService(notifRepo, userRepo, redisCache)
	notifHandler := handler.NewNotificationHandler(notifService)

	blockRepo := repo.NewGormBlockRepo(db)
	blockService := service.NewBlockService(blockRepo, userRepo, redisCache)
	blockHandler := handler.NewBlockHandler(blockService)

	adminRepo := repo.NewGormAdminRepo(db)
	adminService := service.NewAdminService(adminRepo)
	adminHandler := handler.NewAdminHandler(adminService)

	userService := service.NewUserService(userRepo, redisCache, notifService, blockService)
	userHandler := handler.NewUserHandler(userService, jwtService)
	noteRepo := repo.NewGormNoteRepo(db)
	noteService := service.NewNoteService(noteRepo, redisCache, searchRepo, notifService, blockService)
	noteHandler := handler.NewNoteHandler(noteService)
	feedRepo := repo.NewGormFeedRepo(db)
	feedService := service.NewFeedService(feedRepo, userRepo, redisCache, searchRepo, blockService)
	feedHandler := handler.NewFeedHandler(feedService)
	storageService, err := service.NewStorageService(appCfg)
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
	r.GET("/users/:id", userHandler.GetProfile)
	r.GET("/search", feedHandler.Search)
	r.GET("/users/:id/following", jwtService.OptionalJWTAuth(), userHandler.ListFollowing)
	r.GET("/users/:id/followers", jwtService.OptionalJWTAuth(), userHandler.ListFollowers)

	auth := r.Group("/")
	auth.Use(jwtService.JWTAuth())
	{
		auth.GET("/me", userHandler.Me)
		auth.PATCH("/me/updata", userHandler.Updata)
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
	}

	admin := auth.Group("/admin")
	admin.Use(middleware.AdminAuth())
	{
		admin.GET("/users", adminHandler.ListUsers)               // 用户列表
		admin.PATCH("/users/:id/ban", adminHandler.BanUser)       // 封禁/解封
		admin.DELETE("/notes/:id", adminHandler.DeleteNote)       // 删除笔记
		admin.DELETE("/comments/:id", adminHandler.DeleteComment) // 删除评论
		admin.GET("/stats", adminHandler.Stats)                   // 系统统计
	}
	super := auth.Group("/admin")
	super.Use(middleware.SuperAdminAuth())
	{
		super.DELETE("/users/:id", adminHandler.DeleteUser) // 删除用户
	}
	return r
}
