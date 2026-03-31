package handler

import (
	"XFeedSystem/internal/middleware"
	"XFeedSystem/internal/service"
	"net/http"

	"github.com/gin-gonic/gin"
)

type UserHandler struct {
	userService *service.UserService
}
type RegisterRequest struct {
	Username        string `json:"username"`
	Password        string `json:"password"`
	ConfirmPassword string `json:"confirm_password"`
}
type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}
type FollowRequesst struct {
	User_id   int64 `json:"user_id"`
	Follow_id int64 `json:"follow_id"`
}

func NewUserHandler(userService *service.UserService) *UserHandler {
	return &UserHandler{
		userService: userService,
	}
}

func (h *UserHandler) Register(c *gin.Context) {
	var req RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	err := h.userService.Register(req.Username, req.Password, req.ConfirmPassword)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "注册成功",
	})
}
func (h *UserHandler) Login(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	user, err := h.userService.Login(req.Username, req.Password)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	token, err := middleware.GenerateToken(user.ID, user.Username)
	if err != nil {
		c.JSON(500, gin.H{"message": "generate token failed"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "ok",
		"data": gin.H{
			"token": token,
		},
	})
}
func (h *UserHandler) Me(c *gin.Context) {
	uidValue, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"message": "error getting user id from context"})
		return
	}
	uid, ok := uidValue.(int64)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"message": "invalid user id type in context"})
		return
	}
	user, err := h.userService.GetProfile(uid)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "ok",
		"data": gin.H{
			"id":       user.ID,
			"username": user.Username,
		},
	})
}
func (h *UserHandler) Follow(c *gin.Context) {
	var req FollowRequesst
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	ctx := c.Request.Context()
	err := h.userService.Follow(ctx, req.User_id, req.Follow_id)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "ok",
		"data": gin.H{
			"user_id":   req.User_id,
			"follow_id": req.Follow_id,
		},
	})
}
func (h *UserHandler) Unfollow(c *gin.Context) {
	var req FollowRequesst
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	ctx := c.Request.Context()
	err := h.userService.Unfollow(ctx, req.User_id, req.Follow_id)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "ok",
		"data": gin.H{
			"user_id":     req.User_id,
			"unfollow_id": req.Follow_id,
		},
	})
}
func (h *UserHandler) Isfollow(c *gin.Context) {
	var req FollowRequesst
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	}
	ctx := c.Request.Context()
	isfollow, err := h.userService.Isfollow(ctx, req.User_id, req.Follow_id)
	if err != nil {
		c.JSON(500, gin.H{
			"code":    0,
			"message": "error",
		})
		return
	}
	c.JSON(200, gin.H{
		"code":    200,
		"message": "ok",
		"follow":  isfollow,
	})
}
