package handler

import (
	"XFeedSystem/internal/cache"
	"XFeedSystem/internal/service"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

type TopicHandler struct {
	topicService *service.TopicService
	feedService  *service.FeedService
	cache        *cache.RedisCache
}

func NewTopicHandler(t *service.TopicService, f *service.FeedService, c *cache.RedisCache) *TopicHandler {
	return &TopicHandler{topicService: t, feedService: f, cache: c}
}

// Hot GET /topics/hot?limit=20
func (h *TopicHandler) Hot(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	topics, err := h.topicService.Hot(c.Request.Context(), limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 5001, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "ok", "data": topics})
}

// Feed GET /topics/:id/feed?cursor=&limit=
func (h *TopicHandler) Feed(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 4002, "message": "invalid topic id"})
		return
	}
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "10"))
	if limit <= 0 || limit > 50 {
		limit = 10
	}
	cursorStr := c.Query("cursor")

	// 页字节缓存（与 feed 首页同模式，TTL 10s）
	cacheKey := cache.TopicFeedRawKey(id, limit, cursorStr)
	if h.cache != nil {
		if body, err := h.cache.Get(c.Request.Context(), cacheKey); err == nil {
			c.Data(http.StatusOK, "application/json; charset=utf-8", []byte(body))
			return
		}
	}

	currentUserID := getCurrentUserID(c)
	resp, err := h.feedService.ListTopic(c.Request.Context(), id, cursorStr, limit, currentUserID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 5003, "message": err.Error()})
		return
	}
	body, err := json.Marshal(gin.H{"code": 0, "message": "ok", "data": resp})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 5003, "message": err.Error()})
		return
	}
	if h.cache != nil {
		_ = h.cache.Set(c.Request.Context(), cacheKey, string(body), 10*time.Second)
	}
	c.Data(http.StatusOK, "application/json; charset=utf-8", body)
}

// Suggest GET /topics/suggest?q=美食
func (h *TopicHandler) Suggest(c *gin.Context) {
	q := strings.TrimSpace(c.Query("q"))
	if q == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 4003, "message": "q required"})
		return
	}
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "10"))
	if limit <= 0 || limit > 20 {
		limit = 10
	}
	topics, err := h.topicService.Suggest(c.Request.Context(), q, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 5004, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "ok", "data": topics})
}

func (h *TopicHandler) Follow(c *gin.Context) {
	userID, ok := getUserIDFromContext(c)
	if !ok {
		c.JSON(401, gin.H{"code": 4010, "message": " "})
		return
	}
	topicID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || topicID <= 0 {
		c.JSON(400, gin.H{"code": 4000, "message": " "})
		return
	}
	if err := h.topicService.Follow(c.Request.Context(), userID, topicID); err != nil {
		c.JSON(400, gin.H{"code": 4002, "message": err.Error()})
		return
	}
	c.JSON(200, gin.H{"code": 0, "message": "ok"})
}

func (h *TopicHandler) Unfollow(c *gin.Context) {
	userID, ok := getUserIDFromContext(c)
	if !ok {
		c.JSON(401, gin.H{"code": 4010, "message": " "})
		return
	}
	topicID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || topicID <= 0 {
		c.JSON(400, gin.H{"code": 4000, "message": " "})
		return
	}
	if err := h.topicService.Unfollow(c.Request.Context(), userID, topicID); err != nil {
		c.JSON(400, gin.H{"code": 4002, "message": err.Error()})
		return
	}
	c.JSON(200, gin.H{"code": 0, "message": "ok"})
}

func (h *TopicHandler) Followed(c *gin.Context) {
	userID, ok := getUserIDFromContext(c)
	if !ok {
		c.JSON(401, gin.H{"code": 4010, "message": " "})
		return
	}
	topics, err := h.topicService.Followed(c.Request.Context(), userID)
	if err != nil {
		c.JSON(500, gin.H{})
		return
	}
	c.JSON(200, gin.H{"code": 0, "message": "ok", "data": topics})
}
