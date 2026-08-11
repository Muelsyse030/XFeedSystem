package handler

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"XFeedSystem/internal/cache"
	"XFeedSystem/internal/service"

	"github.com/gin-gonic/gin"
)

type FeedHandler struct {
	feedService *service.FeedService
	cache       *cache.RedisCache
}

func NewFeedHandler(feedService *service.FeedService, cache *cache.RedisCache) *FeedHandler {
	return &FeedHandler{feedService: feedService, cache: cache}
}

func (h *FeedHandler) List(c *gin.Context) {
	feedType := c.DefaultQuery("type", "foryou")
	cursorStr := c.Query("cursor")

	limitStr := c.DefaultQuery("limit", "10")
	limit, err := strconv.Atoi(limitStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    4001,
			"message": "invalid limit",
		})
		return
	}
	if limit <= 0 || limit > 50 {
		limit = 10
	}

	currentUserID := getCurrentUserID(c)

	switch feedType {
	case "foryou":
		// 首页/翻页：命中原始字节缓存时零序列化直接返回
		cacheKey := ""
		if h.cache != nil {
			if cursorStr == "" {
				cacheKey = cache.FeedForYouRawKey(currentUserID, limit)
			} else {
				cacheKey = cache.FeedPageRawKey(currentUserID, limit, cursorStr)
			}
			if body, err := h.cache.Get(c.Request.Context(), cacheKey); err == nil {
				c.Data(http.StatusOK, "application/json; charset=utf-8", []byte(body))
				return
			}
		}

		feedList, err := h.feedService.ListForYou(c.Request.Context(), cursorStr, limit, currentUserID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"code":    5001,
				"message": err.Error(),
			})
			return
		}

		resp := gin.H{"code": 0, "message": "ok", "data": feedList}
		body, err := json.Marshal(resp)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"code":    5001,
				"message": err.Error(),
			})
			return
		}
		if h.cache != nil && cacheKey != "" {
			_ = h.cache.Set(c.Request.Context(), cacheKey, string(body), 10*time.Second)
		}
		c.Data(http.StatusOK, "application/json; charset=utf-8", body)
		return

	case "following":
		uidValue, exists := c.Get("userID")
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
				"code":    4011,
				"message": "invalid user id",
			})
			return
		}

		feedList, err := h.feedService.ListFollowing(c.Request.Context(), userID, cursorStr, limit)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"code":    5002,
				"message": err.Error(),
			})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"code":    0,
			"message": "ok",
			"data":    feedList,
		})
		return
	default:
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    4002,
			"message": "unsupported feed type",
		})
		return
	}
}

func (h *FeedHandler) Search(c *gin.Context) {
	keyword := c.Query("q")
	if strings.TrimSpace(keyword) == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    4003,
			"message": "keyword required",
		})
		return
	}
	limitStr := c.DefaultQuery("limit", "10")
	limit, err := strconv.Atoi(limitStr)
	if err != nil || limit <= 0 || limit > 50 {
		limit = 10
	}
	offsetStr := c.DefaultQuery("offset", "0")
	offset, err := strconv.Atoi(offsetStr)
	if err != nil || offset < 0 {
		offset = 0
	}

	currentUserID := getCurrentUserID(c)
	resp, total, err := h.feedService.SearchNotes(c.Request.Context(), keyword, offset, limit, currentUserID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    5003,
			"message": err.Error(),
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "ok",
		"data": gin.H{
			"items":  resp.Items,
			"offset": offset + limit,
			"total":  total,
		},
	})
}
