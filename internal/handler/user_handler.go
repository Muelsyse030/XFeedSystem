package handler

import (
	"XFeedSystem/internal/cache"
	"XFeedSystem/internal/middleware"
	"XFeedSystem/internal/service"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

type UserHandler struct {
	userService *service.UserService
	jwtService  *middleware.JWTService
	cache       *cache.RedisCache
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
type UpdataUserRequest struct {
	AvatarURL string `json:"avatar_url"`
	Bio       string `json:"bio"`
}

func NewUserHandler(userService *service.UserService, jwtService *middleware.JWTService, cache *cache.RedisCache) *UserHandler {
	return &UserHandler{userService: userService, jwtService: jwtService, cache: cache}
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
	token, err := h.jwtService.GenerateToken(user.ID, user.Username, user.Role)
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
			"id":         user.ID,
			"username":   user.Username,
			"avatar_url": user.AvatarURL,
			"bio":        user.Bio,
			"created_at": user.CreatedAt,
			"updated_at": user.UpdatedAt,
			"role":       user.Role,
		},
	})
}

func (h *UserHandler) GetProfile(c *gin.Context) {
	idStr := c.Param("id")
	uid, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || uid <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    4003,
			"message": "invalid user id",
		})
		return
	}
	if h.cache != nil {
		if body, err := h.cache.Get(c.Request.Context(), cache.UserProfileRawKey(uid)); err == nil {
			c.Data(http.StatusOK, "application/json; charset=utf-8", []byte(body))
			return
		}
	}
	user, err := h.userService.GetProfile(uid)
	if err != nil {
		if errors.Is(err, service.ErrUserNotFound) {
			c.JSON(http.StatusNotFound, gin.H{
				"code":    4040,
				"message": "user not found",
			})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    4004,
			"message": err.Error(),
		})
		return
	}
	resp := gin.H{
		"code":    0,
		"message": "ok",
		"data": gin.H{
			"id":         user.ID,
			"username":   user.Username,
			"avatar_url": user.AvatarURL,
			"bio":        user.Bio,
			"created_at": user.CreatedAt,
			"updated_at": user.UpdatedAt,
		},
	}
	body, err := json.Marshal(resp)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 5004, "message": err.Error()})
		return
	}
	if h.cache != nil {
		_ = h.cache.Set(c.Request.Context(), cache.UserProfileRawKey(uid), string(body), 60*time.Second)
	}
	c.Data(http.StatusOK, "application/json; charset=utf-8", body)
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
func (h *UserHandler) Updata(c *gin.Context) {
	var req UpdataUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    4001,
			"message": err.Error(),
		})
		return
	}
	uidValue, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{
			"code":    4010,
			"message": "unauthorized",
		})
		return
	}
	userID, ok := uidValue.(int64)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{
			"code":    4012,
			"message": "invalid user id type",
		})
		return
	}
	err := h.userService.Updata(c.Request.Context(), userID, req.AvatarURL, req.Bio)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    5001,
			"message": err.Error(),
		})
		return
	}
	if h.cache != nil {
		_ = h.cache.Delete(c.Request.Context(), cache.UserProfileRawKey(userID))
	}
	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "ok",
	})
}

func (h *UserHandler) ListFollowing(c *gin.Context) {
	idStr := c.Param("id")
	userID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || userID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 4003, "message": "invalid user id"})
		return
	}
	cursor := c.DefaultQuery("cursor", "")
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "10"))

	currentUserID := getCurrentUserID(c) // 从可选 JWT 中获取，未登录返回 0

	resp, err := h.userService.ListFollowing(c.Request.Context(), userID, cursor, limit, currentUserID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 5001, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "ok", "data": resp})
}

func (h *UserHandler) ListFollowers(c *gin.Context) {
	idStr := c.Param("id")
	userID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || userID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 4003, "message": "invalid user id"})
		return
	}
	cursor := c.DefaultQuery("cursor", "")
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "10"))

	currentUserID := getCurrentUserID(c) // 从可选 JWT 中获取，未登录返回 0

	resp, err := h.userService.ListFollowers(c.Request.Context(), userID, cursor, limit, currentUserID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 5001, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "ok", "data": resp})
}

func getCurrentUserID(c *gin.Context) int64 {
	if v, ok := c.Get("user_id"); ok {
		if uid, ok := v.(int64); ok {
			return uid
		}
	}
	return 0
}
