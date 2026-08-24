package handler

import (
	"XFeedSystem/internal/service"
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

type MessageHandler struct {
	msg *service.MessageService
}

func NewMessageHandler(msg *service.MessageService) *MessageHandler { return &MessageHandler{msg: msg} }

type SendMessageRequest struct {
	ReceiverID      int64  `json:"receiver_id"`
	Content         string `json:"content"`
	ClientMessageID string `json:"client_message_id"`
}

func (h *MessageHandler) Send(c *gin.Context) {
	var req SendMessageRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.ReceiverID <= 0 {
		c.JSON(400, gin.H{"code": 4001, "message": "invalid request"})
		return
	}
	meID, _ := getUserIDFromContext(c)
	msg, err := h.msg.Send(c.Request.Context(), meID, req.ReceiverID, req.Content, req.ClientMessageID)
	if err != nil {
		status := 400
		if err == service.ErrBlocked {
			status = 403
		}
		c.JSON(status, gin.H{"code": 4002, "message": err.Error()})
		return
	}
	c.JSON(200, gin.H{"code": 0, "message": "ok", "data": gin.H{"id": msg.ID, "created_at": msg.CreatedAt}})
}

func (h *MessageHandler) Conversations(c *gin.Context) {
	meID, _ := getUserIDFromContext(c)
	cursor, _ := strconv.ParseInt(c.DefaultQuery("cursor", "0"), 10, 64)
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	list, next, err := h.msg.ListConversations(c.Request.Context(), meID, cursor, limit)
	if err != nil {
		c.JSON(500, gin.H{"code": 5001, "message": err.Error()})
		return
	}
	c.JSON(200, gin.H{"code": 0, "message": "ok", "data": gin.H{"list": list, "next_cursor": next}})
}

func (h *MessageHandler) MarkRead(c *gin.Context) {
	meID, ok := getUserIDFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 4010, "message": "unauthorized"})
		return
	}
	var req struct {
		PeerID int64 `json:"peer_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.PeerID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 4001, "message": "peer_id is required"})
		return
	}
	if err := h.msg.MarkRead(c.Request.Context(), meID, req.PeerID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 5001, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "ok"})
}

func (h *MessageHandler) ListWithPeer(c *gin.Context) {
	meID, ok := getUserIDFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 4010, "message": "unauthorized"})
		return
	}
	peerID, err := strconv.ParseInt(c.Query("peer_id"), 10, 64)
	if err != nil || peerID <= 0 || peerID == meID {
		c.JSON(http.StatusBadRequest, gin.H{"code": 4001, "message": "invalid peer_id"})
		return
	}
	cursor, _ := strconv.ParseInt(c.DefaultQuery("cursor", "0"), 10, 64)
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))

	list, next, err := h.msg.ListWithPeer(c.Request.Context(), meID, peerID, cursor, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 5001, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "ok", "data": gin.H{"list": list, "next_cursor": next}})
}
func (h *MessageHandler) UnreadCount(c *gin.Context) {
	meID, ok := getUserIDFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 4010, "message": "unauthorized"})
		return
	}
	count, err := h.msg.UnreadCount(c.Request.Context(), meID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 5001, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 0, "message": "ok", "data": gin.H{"count": count}})
}
func (h *MessageHandler) Delete(c *gin.Context) {
	idStr := c.Param("id")
	msgID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || msgID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    4001,
			"message": "invalid message id",
		})
		return
	}

	userID, ok := getUserIDFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{
			"code":    4010,
			"message": "unauthorized",
		})
		return
	}

	if err := h.msg.Delete(c.Request.Context(), msgID, userID); err != nil {
		if errors.Is(err, service.ErrMessageNotFound) {
			c.JSON(http.StatusNotFound, gin.H{
				"code":    4040,
				"message": "message not found or not yours",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    5001,
			"message": "delete message failed",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "ok",
	})
}
