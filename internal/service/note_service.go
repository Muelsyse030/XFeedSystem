package service

import (
	"XFeedSystem/internal/cache"
	"XFeedSystem/internal/model"
	"XFeedSystem/internal/pkg/logger"
	"XFeedSystem/internal/repo"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"
)

var (
	ErrInvalidUserID    = errors.New("invalid user id")
	ErrInvalidNoteID    = errors.New("invalid note id")
	ErrInvalidCommentID = errors.New("invalid comment id")
	ErrInvalidComment   = errors.New("invalid comment content")
	ErrNoteNotFound     = errors.New("note not found")
	ErrCommentNotFound  = errors.New("comment not found")
	ErrEmptyNoteContent = errors.New("title and content must not be empty")
	ErrBlocked          = errors.New("无法互动：你已拉黑对方或被对方拉黑")
)

func safeGo(fn func()) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				logger.Sugar.Errorf("goroutine panic recovered: %v", r)
			}
		}()
		fn()
	}()
}

func marshalImages(images []string) string {
	if images == nil {
		return "[]"
	}
	b, _ := json.Marshal(images)
	return string(b)
}

type NoteService struct {
	repo     repo.NoteRepo
	cache    *cache.RedisCache
	search   *repo.SearchRepo
	notifSvc *NotificationService
	block    *BlockService
	topics   *TopicService
}

const noteCacheTTL = 10 * time.Minute
const authorNotesCacheTTL = 5 * time.Minute

func NewNoteService(r repo.NoteRepo, c *cache.RedisCache, sr *repo.SearchRepo, ns *NotificationService, b *BlockService, t *TopicService) *NoteService {
	return &NoteService{repo: r, cache: c, search: sr, notifSvc: ns, block: b, topics: t}
}

// invalidateNoteFeed 笔记级写操作后的缓存失效：
//   - 同步删笔记 JSON + 详情字节缓存（精确 key，立刻生效）
//   - 异步失效全量打分引擎 + feed 页字节缓存（需 SCAN，不阻塞写响应）
func (s *NoteService) invalidateNoteFeed(ctx context.Context, noteID int64) {
	if s.cache == nil {
		return
	}
	_ = s.cache.Delete(ctx, cache.NoteKey(noteID), cache.NoteDetailRawKey(noteID))
	safeGo(func() {
		_ = s.cache.InvalidateFeedEngineAll(context.Background())
		_ = s.cache.InvalidateFeedRawAll(context.Background())
	})
}

func (s *NoteService) Create(userID int64, title, content string, images []string, topics []string) (*model.Note, error) {
	imagesJSON := marshalImages(images)
	note := &model.Note{
		AuthorID:    userID,
		Title:       title,
		Content:     content,
		Images:      imagesJSON,
		Type:        1, //1默认为文章
		PublishedAt: time.Now(),
	}
	if _, err := s.repo.Create(note); err != nil {
		return nil, err
	}
	if s.topics != nil {
		if err := s.topics.AttachToNote(context.Background(), note.ID,
			s.topics.ExtractTopics(content, topics)); err != nil {
			logger.Sugar.Errorf("attach topics err: %v", err)
		}
	}
	if s.cache != nil {
		_ = s.cache.Delete(context.Background(), cache.NoteKey(note.ID), cache.NoteDetailRawKey(note.ID))
		_ = s.cache.Delete(context.Background(),
			cache.UserNotesKey(note.AuthorID, 10),
			cache.UserNotesKey(note.AuthorID, 20),
		)
	}
	safeGo(func() {
		_ = s.cache.InvalidateFeedEngineAll(context.Background())
		_ = s.cache.InvalidateFeedRawAll(context.Background())
	})

	if s.search != nil {
		n := note
		safeGo(func() {
			_ = s.search.Index(context.Background(), &repo.NoteDocument{
				ID:          n.ID,
				AuthorID:    n.AuthorID,
				Title:       n.Title,
				Content:     n.Content,
				Type:        n.Type,
				PublishedAt: n.PublishedAt.Unix(),
			})
		})
	}
	return note, nil
}

func (s *NoteService) ListByAuthorID(ctx context.Context, authorID, cursor int64, limit int) ([]*model.Note, error) {
	if cursor == 0 && s.cache != nil {
		var notes []*model.Note
		if err := s.cache.GetJSON(ctx, cache.UserNotesKey(authorID, limit), &notes); err == nil {
			return notes, nil
		}
	}

	notes, err := s.repo.ListByAuthorID(ctx, authorID, cursor, limit)
	if err != nil {
		return nil, err
	}
	if cursor == 0 && s.cache != nil {
		_ = s.cache.SetJSON(ctx, cache.UserNotesKey(authorID, limit), notes, authorNotesCacheTTL)
	}
	return notes, nil
}

func (s *NoteService) GetByID(ctx context.Context, id int64) (*model.Note, error) {
	if s.cache != nil {
		var note model.Note
		if err := s.cache.GetJSON(ctx, cache.NoteKey(id), &note); err == nil {
			return &note, nil
		}
	}
	n, err := s.repo.GetByID(ctx, id)
	if err != nil {
		logger.Sugar.Errorf("GetByID repo err: %v", err)
		return nil, err
	}
	if s.cache != nil {
		_ = s.cache.SetJSON(ctx, cache.NoteKey(id), n, noteCacheTTL)
	}
	return n, nil
}

func (s *NoteService) Delete(ctx context.Context, id int64, authorID int64) error {
	if err := s.repo.DeleteByID(ctx, id, authorID); err != nil {
		return err
	}
	if s.topics != nil {
		if err := s.topics.DetachFromNote(ctx, id); err != nil {
			logger.Sugar.Errorf("detach topics err: %v", err)
		}
	}
	_ = s.cache.Delete(ctx, cache.NoteKey(id))
	_ = s.cache.Delete(ctx,
		cache.NoteKey(id),
		cache.UserNotesKey(authorID, 10),
		cache.UserNotesKey(authorID, 20),
	)
	if s.search != nil {
		safeGo(func() {
			_ = s.search.Delete(context.Background(), id)
		})
	}
	return nil
}

func (s *NoteService) Like(ctx context.Context, noteID, userID int64) (bool, error) {
	if userID <= 0 {
		return false, ErrInvalidUserID
	}
	note, err := s.repo.GetByID(ctx, noteID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, ErrNoteNotFound
		}
		return false, err
	}
	if err := s.checkBlocked(ctx, userID, note.AuthorID); err != nil {
		return false, err
	}
	created, err := s.repo.Like(ctx, noteID, userID)
	if err == nil {
		s.invalidateNoteFeed(ctx, noteID)
	}
	if created && s.notifSvc != nil {
		note, _ := s.repo.GetByID(ctx, noteID)
		if note != nil {
			n := note
			safeGo(func() {
				s.notifSvc.Create(context.Background(), userID, n.AuthorID, model.NotifTypeLike, noteID, noteID, "赞了你的笔记")
			})
		}
	}
	return created, err
}

func (s *NoteService) Unlike(ctx context.Context, noteID, userID int64) (bool, error) {
	if userID <= 0 {
		return false, ErrInvalidUserID
	}
	if _, err := s.repo.GetByID(ctx, noteID); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, ErrNoteNotFound
		}
		return false, err
	}
	deleted, err := s.repo.Unlike(ctx, noteID, userID)
	if err == nil {
		s.invalidateNoteFeed(ctx, noteID)
	}
	return deleted, err
}

func (s *NoteService) Favorite(ctx context.Context, noteID, userID int64) (bool, error) {
	if userID <= 0 {
		return false, ErrInvalidUserID
	}
	note, err := s.repo.GetByID(ctx, noteID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, ErrNoteNotFound
		}
		return false, err
	}
	if err := s.checkBlocked(ctx, userID, note.AuthorID); err != nil {
		return false, err
	}
	created, err := s.repo.Favorite(ctx, noteID, userID)
	if err == nil {
		s.invalidateNoteFeed(ctx, noteID)
	}
	if created && s.notifSvc != nil {
		note, _ := s.repo.GetByID(ctx, noteID)
		if note != nil {
			n := note
			safeGo(func() {
				s.notifSvc.Create(context.Background(), userID, n.AuthorID, model.NotifTypeFavorite, noteID, noteID, "收藏了你的笔记")
			})
		}
	}
	return created, err
}

func (s *NoteService) Unfavorite(ctx context.Context, noteID, userID int64) (bool, error) {
	if userID <= 0 {
		return false, ErrInvalidUserID
	}
	if _, err := s.repo.GetByID(ctx, noteID); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, ErrNoteNotFound
		}
		return false, err
	}
	deleted, err := s.repo.Unfavorite(ctx, noteID, userID)
		if err == nil {
		s.invalidateNoteFeed(ctx, noteID)
	}
	return deleted, err
}

func (s *NoteService) ListFavorites(ctx context.Context, userID, cursor int64, limit int) ([]*model.Note, int64, error) {
	if userID <= 0 {
		return nil, 0, ErrInvalidUserID
	}
	return s.repo.FavoriteList(ctx, userID, cursor, limit)
}
func (s *NoteService) CreateComment(ctx context.Context, userID, noteID int64, content string) (*model.NoteComment, error) {
	note, err := s.repo.GetByID(ctx, noteID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNoteNotFound
		}
		return nil, err
	}
	if err := s.checkBlocked(ctx, userID, note.AuthorID); err != nil {
		return nil, err
	}
	return s.CreateReply(ctx, userID, noteID, 0, 0, content)
}

func (s *NoteService) CreateReply(ctx context.Context, userID, noteID, parentID, replyToUserID int64, content string) (*model.NoteComment, error) {
	if userID <= 0 {
		return nil, ErrInvalidUserID
	}
	if noteID <= 0 {
		return nil, ErrInvalidNoteID
	}
	if strings.TrimSpace(content) == "" {
		return nil, ErrInvalidComment
	}
	note, err := s.repo.GetByID(ctx, noteID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNoteNotFound
		}
		return nil, err
	}
	if parentID > 0 {
		parent, err := s.repo.GetCommentByID(ctx, parentID)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, ErrCommentNotFound
			}
			return nil, err
		}
		if parent.NoteID != noteID {
			return nil, ErrInvalidCommentID
		}
		if replyToUserID <= 0 {
			replyToUserID = parent.UserID
		}
	}
	comment, err := s.repo.CreateComment(ctx, userID, noteID, parentID, replyToUserID, content)
	if err != nil {
		return nil, err
	}
	s.invalidateNoteFeed(ctx, noteID)

	if s.notifSvc != nil {
		n := note
		if parentID == 0 {
			safeGo(func() {
				s.notifSvc.Create(context.Background(), userID, n.AuthorID,
					model.NotifTypeComment, comment.ID, noteID, "评论了你的笔记")
			})
		} else {
			safeGo(func() {
				s.notifSvc.Create(context.Background(), userID, replyToUserID,
					model.NotifTypeReply, comment.ID, noteID, "回复了你")
			})
		}
	}
	return comment, nil
}

func (s *NoteService) ListCommentsByNoteID(ctx context.Context, noteID, cursor int64, limit int) ([]*model.NoteComment, error) {
	if noteID <= 0 {
		return nil, ErrInvalidNoteID
	}
	if limit <= 0 || limit > 50 {
		limit = 10
	}
	return s.repo.ListCommentsByNoteID(ctx, noteID, cursor, limit)
}

func (s *NoteService) ListRepliesByParentID(ctx context.Context, noteID, parentID int64, limit int) ([]*model.NoteComment, error) {
	if noteID <= 0 {
		return nil, ErrInvalidNoteID
	}
	if parentID <= 0 {
		return nil, ErrInvalidCommentID
	}
	if limit <= 0 || limit > 50 {
		limit = 10
	}
	return s.repo.ListRepliesByParentID(ctx, noteID, parentID, limit)
}
func (s *NoteService) DeleteComment(ctx context.Context, commentID int64, userID int64) error {
	if commentID <= 0 {
		return ErrInvalidCommentID
	}
	comment, err := s.repo.GetCommentByID(ctx, commentID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrCommentNotFound
		}
		return err
	}
	if err := s.repo.DeleteComment(ctx, commentID, userID); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrCommentNotFound
		}
		return err
	}
	s.invalidateNoteFeed(ctx, comment.NoteID)
	return nil
}
func (s *NoteService) Updata(ctx context.Context, noteID, authorID int64, title, content string, images []string, topics []string) error {
	if noteID <= 0 {
		return ErrInvalidNoteID
	}
	if authorID <= 0 {
		return ErrInvalidUserID
	}
	if strings.TrimSpace(title) == "" || strings.TrimSpace(content) == "" {
		return ErrEmptyNoteContent
	}
	if err := s.repo.UpdataByAuthorID(ctx, noteID, authorID, title, content, marshalImages(images)); err != nil {
		return err
	}
	if s.topics != nil {
		if err := s.topics.ReplaceTopics(ctx, noteID, s.topics.ExtractTopics(content, topics)); err != nil {
			logger.Sugar.Errorf("replace topics err: %v", err)
		}
	}
	_ = s.cache.Delete(ctx, cache.NoteKey(noteID))
	_ = s.cache.Delete(ctx, cache.NoteDetailRawKey(noteID))
	_ = s.cache.Delete(ctx,
		cache.NoteKey(noteID),
		cache.UserNotesKey(authorID, 10),
		cache.UserNotesKey(authorID, 20),
	)
	if s.search != nil {
		safeGo(func() {
			_, err := s.repo.GetByID(ctx, noteID)
			if err != nil {
				return
			}
			_ = s.search.Index(context.Background(), &repo.NoteDocument{
				ID:          noteID,
				Title:       title,
				Content:     content,
				AuthorID:    authorID,
				PublishedAt: time.Now().Unix(),
			})
		})
	}

	safeGo(func() {
		_ = s.cache.InvalidateFeedEngineAll(context.Background())
		_ = s.cache.InvalidateFeedRawAll(context.Background())
	})
	
	return nil
}

func (s *NoteService) IsLiked(ctx context.Context, noteID, userID int64) (bool, error) {
	if userID <= 0 {
		return false, nil
	}
	return s.repo.IsLiked(ctx, noteID, userID)
}

func (s *NoteService) IsFavorite(ctx context.Context, noteID, userID int64) (bool, error) {
	if userID <= 0 {
		return false, nil
	}
	return s.repo.IsFavorite(ctx, noteID, userID)
}

func (s *NoteService) checkBlocked(ctx context.Context, currentUserID, targetUserID int64) error {
	if s.block == nil {
		return nil
	}
	blocked, err := s.block.IsBlockedEitherWay(ctx, currentUserID, targetUserID)
	if err != nil {
		return err
	}
	if blocked {
		return ErrBlocked
	}
	return nil
}
