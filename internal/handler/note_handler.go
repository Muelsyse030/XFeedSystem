package handler

import (
	"XFeedSystem/internal/service"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

type NoteHandler struct {
	noteService *service.NoteService
}
type CreateNoteRequest struct {
	Title   string `json:"title"`
	Content string `json:"content"`
}
type NoteResponse struct {
	ID          int64     `json:"id"`
	AuthorID    int64     `json:"author_id"`
	Title       string    `json:"title"`
	Content     string    `json:"content"`
	PublishedAt time.Time `json:"published_at"`
	CreatedAt   time.Time `json:"created_at"`
}

func NewNoteHandler(noteService *service.NoteService) *NoteHandler {
	return &NoteHandler{
		noteService: noteService,
	}
}

func (h *NoteHandler) Create(c *gin.Context) {
	var req CreateNoteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{
			"code":    4001,
			"message": err.Error(),
		})
		return
	}
	uidValue, exists := c.Get("user_id")
	if !exists {
		c.JSON(401, gin.H{
			"code":    4002,
			"message": "用户未登录",
		})
		return
	}
	userID, ok := uidValue.(int64)
	if !ok {
		c.JSON(500, gin.H{
			"code":    5001,
			"message": "用户ID类型错误",
		})
		return
	}
	note, err := h.noteService.Create(userID, req.Title, req.Content)
	if err != nil {
		c.JSON(500, gin.H{
			"code":    5002,
			"message": err.Error(),
		})
		return
	}
	c.JSON(200, gin.H{
		"code":    200,
		"message": "ok",
		"data": gin.H{
			"id":           note.ID,
			"author_id":    note.AuthorID,
			"title":        note.Title,
			"content":      note.Content,
			"type":         note.Type,
			"published_at": note.PublishedAt,
			"created_at":   note.CreatedAt,
		},
	})
}

func (h *NoteHandler) ListByUser(c *gin.Context) {
	idStr := c.Param("id")
	authorID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    4003,
			"message": "invalid user id",
		})
		return
	}
	cursorStr := c.DefaultQuery("cursor", "0")
	limitStr := c.DefaultQuery("limit", "10")
	cursor, _ := strconv.ParseInt(cursorStr, 10, 64)
	limit, _ := strconv.Atoi(limitStr)
	notes, err := h.noteService.ListByAuthorID(c.Request.Context(), authorID, cursor, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    5003,
			"message": "list notes failed",
		})
		return
	}

	resp := make([]gin.H, 0, len(notes))
	var nextCursor int64 = 0
	for _, note := range notes {
		resp = append(resp, gin.H{
			"id":           note.ID,
			"author_id":    note.AuthorID,
			"title":        note.Title,
			"content":      note.Content,
			"published_at": note.PublishedAt,
			"created_at":   note.CreatedAt,
		})
		nextCursor = note.ID
	}
	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "ok",
		"data": gin.H{
			"list":        resp,
			"next_cursor": nextCursor,
		},
	})
}
func (h *NoteHandler) Detail(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    4002,
			"message": "invalid note id",
		})
		return
	}
	note, err := h.noteService.GetByID(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    5004,
			"message": "get note failed",
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "ok",
		"data": gin.H{
			"id":           note.ID,
			"author_id":    note.AuthorID,
			"title":        note.Title,
			"content":      note.Content,
			"published_at": note.PublishedAt,
			"created_at":   note.CreatedAt,
		},
	})
}

func (h *NoteHandler) Delete(c *gin.Context) {
	idStr := c.Param("id")
	noteID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    4002,
			"message": "invalid note id",
		})
		return
	}

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

	if err := h.noteService.Delete(c.Request.Context(), noteID, userID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    5002,
			"message": "delete note failed",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "ok",
	})
}
