package handler

import (
	"XFeedSystem/internal/service"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

type BlockHandler struct {
	blockService *service.BlockService
}

func NewBlockHandler(bs *service.BlockService) *BlockHandler {
	return &BlockHandler{blockService: bs}
}

func (h *BlockHandler) Block(c *gin.Context) {
	targetID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || targetID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 4003, "message": "invalid user id"})
		return
	}
	userID := c.GetInt64("user_id")

	if err := h.blockService.Block(c.Request.Context(), userID, targetID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 4004, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "ok"})
}

func (h *BlockHandler) Unblock(c *gin.Context) {
	targetID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || targetID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 4003, "message": "invalid user id"})
		return
	}
	userID := c.GetInt64("user_id")

	if err := h.blockService.Unblock(c.Request.Context(), userID, targetID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 4004, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "ok"})
}
