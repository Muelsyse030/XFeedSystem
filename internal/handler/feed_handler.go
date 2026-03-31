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
	feedtype := c.DefaultQuery("type", "foryou")
	if feedtype != "foryou" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    "4002",
			"message": "invalid feed type",
		})
		return
	}
	limitStr := c.DefaultQuery("limit", "10")
	limit, err := strconv.Atoi(limitStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    4001,
			"message": "invalid limit",
		})
		return
	}

	feedList, err := h.feedService.List(c.Request.Context(), c.Query("cursor"), limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    5001,
			"message": err.Error(),
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"code": 0,
		"message": "OK",
		"data": feedList,
	})
}
