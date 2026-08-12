package handler

import (
	"XFeedSystem/internal/cache"
	"XFeedSystem/internal/model"
	"XFeedSystem/internal/repo"
	"XFeedSystem/internal/service"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type NoteHandler struct {
	noteService *service.NoteService
	userRepo    *repo.GormUserRepo
	cache       *cache.RedisCache
}
type CreateNoteRequest struct {
	Title   string   `json:"title"`
	Content string   `json:"content"`
	Type    int      `json:"type"`
	Images  []string `json:"images"`
	Topics  []string `json:"topics"`
}
type NoteResponse struct {
	ID          int64     `json:"id"`
	AuthorID    int64     `json:"author_id"`
	Title       string    `json:"title"`
	Content     string    `json:"content"`
	PublishedAt time.Time `json:"published_at"`
	CreatedAt   time.Time `json:"created_at"`
}

func NewNoteHandler(noteService *service.NoteService, userRepo *repo.GormUserRepo, cache *cache.RedisCache) *NoteHandler {
	return &NoteHandler{noteService: noteService, userRepo: userRepo, cache: cache}
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
	note, err := h.noteService.Create(userID, req.Title, req.Content, req.Images, req.Topics)
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
			"images":       parseImages(note.Images),
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
			"images":       parseImages(note.Images),
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

	userID, _ := getUserIDFromContext(c)

	// 匿名详情：命中原始字节缓存直接返回，零序列化
	if userID == 0 && h.cache != nil {
		cacheKey := cache.NoteDetailRawKey(id)
		if body, err := h.cache.Get(c.Request.Context(), cacheKey); err == nil {
			c.Data(http.StatusOK, "application/json; charset=utf-8", []byte(body))
			return
		}
	}

	note, err := h.noteService.GetByID(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    5004,
			"message": "get note failed",
		})
		return
	}

	isLiked := false
	isFavorited := false
	if userID > 0 {
		if liked, err := h.noteService.IsLiked(c.Request.Context(), note.ID, userID); err == nil {
			isLiked = liked
		}
		if faved, err := h.noteService.IsFavorite(c.Request.Context(), note.ID, userID); err == nil {
			isFavorited = faved
		}
	}

	resp := gin.H{
		"code":    0,
		"message": "ok",
		"data": gin.H{
			"id":             note.ID,
			"author_id":      note.AuthorID,
			"title":          note.Title,
			"content":        note.Content,
			"images":         parseImages(note.Images),
			"published_at":   note.PublishedAt,
			"created_at":     note.CreatedAt,
			"like_count":     note.LikeCount,
			"favorite_count": note.FavoriteCount,
			"comment_count":  note.CommentCount,
			"is_liked":       isLiked,
			"is_favorited":   isFavorited,
		},
	}
	body, err := json.Marshal(resp)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 5004, "message": err.Error()})
		return
	}
	if userID == 0 && h.cache != nil {
		_ = h.cache.Set(c.Request.Context(), cache.NoteDetailRawKey(id), string(body), 30*time.Second)
	}
	c.Data(http.StatusOK, "application/json; charset=utf-8", body)
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
	created, err := h.noteService.Like(c.Request.Context(), noteID, userID)
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
		"data": gin.H{
			"created": created,
		},
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
	deleted, err := h.noteService.Unlike(c.Request.Context(), noteID, userID)
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
		"data": gin.H{
			"deleted": deleted,
		},
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
	created, err := h.noteService.Favorite(c.Request.Context(), noteID, userID)
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
		"data": gin.H{
			"created": created,
		},
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
	deleted, err := h.noteService.Unfavorite(c.Request.Context(), noteID, userID)
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
		"data": gin.H{
			"deleted": deleted,
		},
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
			"images":       parseImages(note.Images),
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
		Content       string `json:"content"`
		ParentID      int64  `json:"parent_id"`
		ReplyToUserID int64  `json:"reply_to_user_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    4003,
			"message": "invalid request",
		})
		return
	}
	req.Content = strings.TrimSpace(req.Content)
	comment, err := h.noteService.CreateReply(c.Request.Context(), userID, noteID, req.ParentID, req.ReplyToUserID, req.Content)
	if err != nil {
		if errors.Is(err, service.ErrInvalidComment) || errors.Is(err, service.ErrInvalidCommentID) || errors.Is(err, service.ErrInvalidNoteID) {
			c.JSON(http.StatusBadRequest, gin.H{
				"code":    4003,
				"message": err.Error(),
			})
			return
		}
		if errors.Is(err, service.ErrNoteNotFound) || errors.Is(err, service.ErrCommentNotFound) {
			c.JSON(http.StatusNotFound, gin.H{
				"code":    4040,
				"message": err.Error(),
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    5001,
			"message": "create comment failed",
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "ok",
		"data":    h.buildCommentItem(c.Request.Context(), comment, noteID),
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

	repliesByComment := make(map[int64][]*model.NoteComment, len(comments))
	userIDs := make(map[int64]struct{})
	for _, cm := range comments {
		userIDs[cm.UserID] = struct{}{}
		replies, err := h.noteService.ListRepliesByParentID(c.Request.Context(), noteID, cm.ID, 50)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"code":    5002,
				"message": "list replies failed",
			})
			return
		}
		repliesByComment[cm.ID] = replies
		for _, rp := range replies {
			userIDs[rp.UserID] = struct{}{}
		}
	}

	userMap := make(map[int64]*model.User, len(userIDs))
	if len(userIDs) > 0 {
		idList := make([]int64, 0, len(userIDs))
		for id := range userIDs {
			idList = append(idList, id)
		}
		users, err := h.userRepo.GetByIDs(idList)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"code":    5002,
				"message": "list comments failed",
			})
			return
		}
		for _, u := range users {
			userMap[u.ID] = u
		}
	}

	resp := make([]gin.H, 0, len(comments))
	for _, cm := range comments {
		replyResp := make([]gin.H, 0, len(repliesByComment[cm.ID]))
		for _, rp := range repliesByComment[cm.ID] {
			reply := gin.H{
				"id":               rp.ID,
				"note_id":          rp.NoteID,
				"user_id":          rp.UserID,
				"parent_id":        rp.ParentID,
				"reply_to_user_id": rp.ReplyToUserID,
				"content":          rp.Content,
				"created_at":       rp.CreatedAt,
			}
			if u, ok := userMap[rp.UserID]; ok {
				reply["user"] = gin.H{
					"id":         u.ID,
					"username":   u.Username,
					"avatar_url": u.AvatarURL,
				}
			}
			replyResp = append(replyResp, reply)
		}
		comment := gin.H{
			"id":         cm.ID,
			"note_id":    cm.NoteID,
			"user_id":    cm.UserID,
			"content":    cm.Content,
			"created_at": cm.CreatedAt,
			"replies":    replyResp,
		}
		if u, ok := userMap[cm.UserID]; ok {
			comment["user"] = gin.H{
				"id":         u.ID,
				"username":   u.Username,
				"avatar_url": u.AvatarURL,
			}
		}
		resp = append(resp, comment)
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

func (h *NoteHandler) buildCommentItem(ctx context.Context, cm *model.NoteComment, noteID int64) gin.H {
	item := gin.H{
		"id":               cm.ID,
		"note_id":          cm.NoteID,
		"user_id":          cm.UserID,
		"parent_id":        cm.ParentID,
		"reply_to_user_id": cm.ReplyToUserID,
		"content":          cm.Content,
		"created_at":       cm.CreatedAt,
		"replies":          []gin.H{},
	}
	if cm.ParentID == 0 {
		replies, err := h.noteService.ListRepliesByParentID(ctx, noteID, cm.ID, 50)
		if err == nil {
			replyResp := make([]gin.H, 0, len(replies))
			for _, rp := range replies {
				replyResp = append(replyResp, gin.H{
					"id":               rp.ID,
					"note_id":          rp.NoteID,
					"user_id":          rp.UserID,
					"parent_id":        rp.ParentID,
					"reply_to_user_id": rp.ReplyToUserID,
					"content":          rp.Content,
					"created_at":       rp.CreatedAt,
				})
			}
			item["replies"] = replyResp
		}
	}
	return item
}

func (h *NoteHandler) Updata(c *gin.Context) {
	idStr := c.Param("id")
	noteID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || noteID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    4002,
			"message": "invalid note id",
		})
		return
	}
	var req CreateNoteRequest
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
	err = h.noteService.Updata(c.Request.Context(), noteID, userID, req.Title, req.Content, req.Images, req.Topics)
	if err != nil {
		if errors.Is(err, service.ErrEmptyNoteContent) {
			c.JSON(http.StatusBadRequest, gin.H{
				"code":    4003,
				"message": err.Error(),
			})
			return
		}
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{
				"code":    4040,
				"message": "note not found or not editable",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    5001,
			"message": err.Error(),
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "ok",
	})
}

func parseImages(imagesJSON string) []string {
	if imagesJSON == "" {
		return []string{}
	}
	var urls []string
	if err := json.Unmarshal([]byte(imagesJSON), &urls); err != nil {
		return []string{}
	}
	return urls
}
