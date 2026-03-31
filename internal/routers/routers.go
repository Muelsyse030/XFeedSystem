package routers

import (
	"XFeedSystem/internal/handler"
	"XFeedSystem/internal/middleware"
	"XFeedSystem/internal/repo"
	"XFeedSystem/internal/service"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func SetupRouter(db *gorm.DB) *gin.Engine {

	r := gin.Default()
	userRepo := repo.NewGormUserRepo(db)
	userService := service.NewUserService(userRepo)
	userHandler := handler.NewUserHandler(userService)
	noteRepo := repo.NewGormNoteRepo(db)
	noteService := service.NewNoteService(noteRepo)
	noteHandler := handler.NewNoteHandler(noteService)
	feedRepo := repo.NewGormFeedRepo(db)
	feedService := service.NewFeedService(feedRepo, userRepo)
	feedHandler := handler.NewFeedHandler(feedService)

	r.GET("/ping", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"message": "pong",
		})
	})

	r.POST("/register", userHandler.Register)
	r.POST("/login", userHandler.Login)

	r.GET("/notes/:id", noteHandler.Detail)
	r.GET("/users/:id/notes", noteHandler.ListByUser)
	r.GET("/feed", feedHandler.List)
	auth := r.Group("/")
	auth.Use(middleware.JWTAuth())
	{
		auth.GET("/me", userHandler.Me)
		auth.POST("/notes", noteHandler.Create)
		auth.DELETE("/notes/:id", noteHandler.Delete)
		auth.POST("/users/:id/follow", userHandler.Follow)
		auth.DELETE("users/:id/unfollow", userHandler.Unfollow)
		auth.POST("/users/:id/isfollow", userHandler.Isfollow)
	}
	return r
}
