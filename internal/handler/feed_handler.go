package handler

import (
	"XFeedSystem/internal/service"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

type FeedHandler struct {
	feedService *service.FeedService
}

func NewFeedHandler(feedService *service.FeedService) *FeedHandler {
	return &FeedHandler{
		feedService: feedService,
	}
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

	switch feedType {
	case "foryou":
		feedList, err := h.feedService.ListForYou(c.Request.Context(), cursorStr, limit)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"code":    5001,
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
