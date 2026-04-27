package handler

import (
	"XFeedSystem/internal/service"
	"errors"
	"net/http"
	"strconv"
	"strings"
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

	userID, ok := getUserIDFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{
			"code":    4010,
			"message": "unauthorized",
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

func getUserIDFromContext(c *gin.Context) (int64, bool) {
	if v, ok := c.Get("user_id"); ok {
		if uid, ok2 := v.(int64); ok2 {
			return uid, true
		}
	}
	if v, ok := c.Get("userID"); ok {
		if uid, ok2 := v.(int64); ok2 {
			return uid, true
		}
	}
	return 0, false
}

func (h *NoteHandler) Like(c *gin.Context) {
	idStr := c.Param("id")
	noteID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    4002,
			"message": "invalid note id",
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
	_, err = h.noteService.Like(c.Request.Context(), noteID, userID)
	if err != nil {
		if errors.Is(err, service.ErrInvalidUserID) {
			c.JSON(http.StatusBadRequest, gin.H{
				"code":    4003,
				"message": "invalid user id",
			})
			return
		}
		if errors.Is(err, service.ErrNoteNotFound) {
			c.JSON(http.StatusNotFound, gin.H{
				"code":    4040,
				"message": "note not found",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    5005,
			"message": "like note failed",
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "ok",
	})
}

func (h *NoteHandler) Unlike(c *gin.Context) {
	idStr := c.Param("id")
	noteID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    4002,
			"message": "invalid note id",
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
	err = h.noteService.Unlike(c.Request.Context(), noteID, userID)
	if err != nil {
		if errors.Is(err, service.ErrInvalidUserID) {
			c.JSON(http.StatusBadRequest, gin.H{
				"code":    4003,
				"message": "invalid user id",
			})
			return
		}
		if errors.Is(err, service.ErrNoteNotFound) {
			c.JSON(http.StatusNotFound, gin.H{
				"code":    4040,
				"message": "note not found",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    5006,
			"message": "unlike note failed",
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "ok",
	})
}

func (h *NoteHandler) Favorite(c *gin.Context) {
	idStr := c.Param("id")
	noteID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    4002,
			"message": "invalid note id",
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
	_, err = h.noteService.Favorite(c.Request.Context(), noteID, userID)
	if err != nil {
		if errors.Is(err, service.ErrInvalidUserID) {
			c.JSON(http.StatusBadRequest, gin.H{
				"code":    4003,
				"message": "invalid user id",
			})
			return
		}
		if errors.Is(err, service.ErrNoteNotFound) {
			c.JSON(http.StatusNotFound, gin.H{
				"code":    4040,
				"message": "note not found",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    5007,
			"message": "favorite note failed",
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "ok",
	})
}

func (h *NoteHandler) Unfavorite(c *gin.Context) {
	idStr := c.Param("id")
	noteID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    4002,
			"message": "invalid note id",
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
	err = h.noteService.Unfavorite(c.Request.Context(), noteID, userID)
	if err != nil {
		if errors.Is(err, service.ErrInvalidUserID) {
			c.JSON(http.StatusBadRequest, gin.H{
				"code":    4003,
				"message": "invalid user id",
			})
			return
		}
		if errors.Is(err, service.ErrNoteNotFound) {
			c.JSON(http.StatusNotFound, gin.H{
				"code":    4040,
				"message": "note not found",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    5008,
			"message": "unfavorite note failed",
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "ok",
	})
}

func (h *NoteHandler) ListFavorites(c *gin.Context) {
	userID, ok := getUserIDFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{
			"code":    4010,
			"message": "unauthorized",
		})
		return
	}
	cursorStr := c.DefaultQuery("cursor", "0")
	limitStr := c.DefaultQuery("limit", "10")
	cursor, _ := strconv.ParseInt(cursorStr, 10, 64)
	limit, _ := strconv.Atoi(limitStr)

	notes, nextCursor, err := h.noteService.ListFavorites(c.Request.Context(), userID, cursor, limit)
	if err != nil {
		if errors.Is(err, service.ErrInvalidUserID) {
			c.JSON(http.StatusBadRequest, gin.H{
				"code":    4003,
				"message": "invalid user id",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    5009,
			"message": "list favorites failed",
		})
		return
	}

	resp := make([]gin.H, 0, len(notes))
	for _, note := range notes {
		resp = append(resp, gin.H{
			"id":           note.ID,
			"author_id":    note.AuthorID,
			"title":        note.Title,
			"content":      note.Content,
			"published_at": note.PublishedAt,
			"created_at":   note.CreatedAt,
		})
	}
	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "ok",
		"data": gin.H{
			"list":        resp,
			"next_cursor": nextCursor,
		},
	})
}
func (h *NoteHandler) Comment(c *gin.Context) {
	idStr := c.Param("id")
	noteID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    4002,
			"message": "invalid note id",
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
	var req struct {
		Content string `json:"content"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    4003,
			"message": "invalid request",
		})
		return
	}
	req.Content = strings.TrimSpace(req.Content)
	if req.Content == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    4003,
			"message": "content required",
		})
		return
	}
	comment, err := h.noteService.CreateComment(c.Request.Context(), userID, noteID, req.Content)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    5001,
			"message": "create comment failed",
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "ok",
		"data": gin.H{
			"id":      comment.ID,
			"note_id": comment.NoteID,
			"user_id": comment.UserID,
		},
	})
}
func (h *NoteHandler) ListComments(c *gin.Context) {
	idStr := c.Param("id")
	noteID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    4002,
			"message": "invalid note id",
		})
		return
	}

	cursorStr := c.DefaultQuery("cursor", "0")
	cursor, err := strconv.ParseInt(cursorStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    4003,
			"message": "invalid cursor",
		})
		return
	}

	limitStr := c.DefaultQuery("limit", "10")
	limit, err := strconv.Atoi(limitStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    4004,
			"message": "invalid limit",
		})
		return
	}

	comments, err := h.noteService.ListCommentsByNoteID(c.Request.Context(), noteID, cursor, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    5002,
			"message": "list comments failed",
		})
		return
	}

	nextCursor := int64(0)
	if len(comments) > 0 {
		nextCursor = comments[len(comments)-1].ID
	}

	resp := make([]gin.H, 0, len(comments))
	for _, cm := range comments {
		resp = append(resp, gin.H{
			"id":         cm.ID,
			"note_id":    cm.NoteID,
			"user_id":    cm.UserID,
			"content":    cm.Content,
			"created_at": cm.CreatedAt,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "ok",
		"data": gin.H{
			"list":        resp,
			"next_cursor": nextCursor,
		},
	})
}
func (h *NoteHandler) DeleteComment(c *gin.Context) {
	commentIDStr := c.Param("comment_id")
	commentID, err := strconv.ParseInt(commentIDStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    4002,
			"message": "invalid comment id",
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

	if err := h.noteService.DeleteComment(c.Request.Context(), commentID, userID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    4005,
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "ok",
	})
}
