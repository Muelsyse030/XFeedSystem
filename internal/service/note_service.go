package service

import (
	"XFeedSystem/internal/cache"
	"XFeedSystem/internal/model"
	"XFeedSystem/internal/pkg/cursor"
	"XFeedSystem/internal/pkg/logger"
	"XFeedSystem/internal/repo"
	"context"
	"encoding/json"
	"errors"
	"regexp"
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
	ErrContentTooLong   = errors.New("content too long")
	ErrVersionNotFound  = errors.New("version not found")
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
	repo   repo.NoteRepo
	cache  *cache.RedisCache
	block  *BlockService
	topics *TopicService
	user   repo.UserRepo
}

const noteCacheTTL = 10 * time.Minute
const authorNotesCacheTTL = 5 * time.Minute

var mentionRe = regexp.MustCompile(`@([\p{L}\p{N}_-]{2,32})`)

func extractMentions(content string) []string {
	seen := map[string]bool{}
	var out []string
	for _, m := range mentionRe.FindAllStringSubmatch(content, -1) {
		name := m[1]
		if !seen[name] {
			seen[name] = true
			out = append(out, name)
		}
	}
	return out
}

func NewNoteService(r repo.NoteRepo, c *cache.RedisCache,
	b *BlockService, t *TopicService, us repo.UserRepo) *NoteService {
	return &NoteService{repo: r, cache: c, block: b, topics: t, user: us}
}

// invalidateNoteFeed 笔记级写操作后的缓存失效：
// 只删精确的笔记 JSON + 详情字节缓存；打分 ZSET / feed 页缓存由 Feed Worker 消费事件处理。
func (s *NoteService) invalidateNoteFeed(ctx context.Context, noteID int64) {
	if s.cache != nil {
		_ = s.cache.Delete(ctx, cache.NoteKey(noteID), cache.NoteDetailRawKey(noteID), cache.FeedNoteKey(noteID))
	}
}

// normalizeNoteType 笔记类型：1=图文（默认），2=视频
func normalizeNoteType(t int) int8 {
	if t == 2 {
		return 2
	}
	return 1
}

func (s *NoteService) Create(userID int64, title, content string, images []string, topics []string, noteType int, videoURL string, contentFormat int) (*model.Note, error) {
	normalized, format := NormalizeContent(content, contentFormat)
	if err := ValidateNoteContent(title, normalized, format); err != nil {
		return nil, err
	}
	// 没单独上传图片时，自动把正文第一张图作为封面
	if len(images) == 0 && format == ContentFormatRich {
		if cover := ExtractFirstImage(normalized); cover != "" {
			images = []string{cover}
		}
	}

	imagesJSON := marshalImages(images)
	note := &model.Note{
		AuthorID:      userID,
		Title:         strings.TrimSpace(title),
		Content:       normalized,
		Images:        imagesJSON,
		Type:          normalizeNoteType(noteType),
		VideoURL:      strings.TrimSpace(videoURL),
		ContentFormat: format,
		PublishedAt:   time.Now(),
	}
	if _, err := s.repo.Create(context.Background(), note, extractMentions(cursor.StripHTML(normalized))); err != nil {
		return nil, err
	}

	// 话题识别用纯文本，避免匹配到 HTML 属性里的 #xx
	if s.topics != nil {
		if err := s.topics.AttachToNote(context.Background(), note.ID,
			s.topics.ExtractTopics(cursor.StripHTML(normalized), topics)); err != nil {
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
	return nil
}

func (s *NoteService) Like(ctx context.Context, noteID, userID int64) (bool, error) {
	if userID <= 0 {
		return false, ErrInvalidUserID
	}
	note, err := s.GetByID(ctx, noteID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, ErrNoteNotFound
		}
		return false, err
	}
	if err := s.checkBlocked(ctx, userID, note.AuthorID); err != nil {
		return false, err
	}
	created, err := s.repo.Like(ctx, noteID, userID, note.AuthorID)
	if err != nil {
		return created, err
	}
	s.invalidateNoteFeed(ctx, noteID)
	return created, nil
}

func (s *NoteService) Unlike(ctx context.Context, noteID, userID int64) (bool, error) {
	if userID <= 0 {
		return false, ErrInvalidUserID
	}
	note, err := s.GetByID(ctx, noteID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, ErrNoteNotFound
		}
		return false, err
	}
	deleted, err := s.repo.Unlike(ctx, noteID, userID, note.AuthorID)
	if err != nil {
		return deleted, err
	}
	s.invalidateNoteFeed(ctx, noteID)
	return deleted, err
}

func (s *NoteService) Favorite(ctx context.Context, noteID, userID int64) (bool, error) {
	if userID <= 0 {
		return false, ErrInvalidUserID
	}
	note, err := s.GetByID(ctx, noteID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, ErrNoteNotFound
		}
		return false, err
	}
	if err := s.checkBlocked(ctx, userID, note.AuthorID); err != nil {
		return false, err
	}
	created, err := s.repo.Favorite(ctx, noteID, userID, note.AuthorID)
	if err != nil {
		return created, err
	}
	s.invalidateNoteFeed(ctx, noteID)
	return created, err
}

func (s *NoteService) Unfavorite(ctx context.Context, noteID, userID int64) (bool, error) {
	if userID <= 0 {
		return false, ErrInvalidUserID
	}
	note, err := s.GetByID(ctx, noteID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, ErrNoteNotFound
		}
		return false, err
	}
	deleted, err := s.repo.Unfavorite(ctx, noteID, userID, note.AuthorID)
	if err != nil {
		return deleted, err
	}
	s.invalidateNoteFeed(ctx, noteID)
	return deleted, err
}

func (s *NoteService) ListFavorites(ctx context.Context, userID, cursor int64, limit int) ([]*model.Note, int64, error) {
	if userID <= 0 {
		return nil, 0, ErrInvalidUserID
	}
	return s.repo.FavoriteList(ctx, userID, cursor, limit)
}
func (s *NoteService) CreateComment(ctx context.Context, userID, noteID int64, content string) (*model.NoteComment, error) {
	note, err := s.GetByID(ctx, noteID)
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
	note, err := s.GetByID(ctx, noteID)
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
	comment, err := s.repo.CreateComment(ctx, userID, noteID, parentID, replyToUserID, content,
		note.AuthorID, extractMentions(content))
	if err != nil {
		return nil, err
	}
	s.invalidateNoteFeed(ctx, noteID)
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
func (s *NoteService) Updata(ctx context.Context, noteID, authorID int64, title, content string, images []string, topics []string, noteType int, videoURL string, contentFormat int) error {
	if noteID <= 0 {
		return ErrInvalidNoteID
	}
	if authorID <= 0 {
		return ErrInvalidUserID
	}
	normalized, format := NormalizeContent(content, contentFormat)
	if err := ValidateNoteContent(title, normalized, format); err != nil {
		return err
	}
	// 没单独上传图片时，自动把正文第一张图作为封面
	if len(images) == 0 && format == ContentFormatRich {
		if cover := ExtractFirstImage(normalized); cover != "" {
			images = []string{cover}
		}
	}
	if old, err := s.GetByID(ctx, noteID); err == nil && old.AuthorID == authorID {
		_ = s.repo.InsertNoteVersion(ctx, &model.NoteVersion{
			NoteID:        old.ID,
			AuthorID:      old.AuthorID,
			Title:         old.Title,
			Content:       old.Content,
			Images:        old.Images,
			VideoURL:      old.VideoURL,
			Type:          old.Type,
			ContentFormat: old.ContentFormat,
			CreatedAt:     time.Now(),
		})
		_ = s.repo.TrimNoteVersions(ctx, noteID, 50) // 只保留最近 50 个版本
	}

	if err := s.repo.UpdataByAuthorID(ctx, noteID, authorID,
		strings.TrimSpace(title), normalized, marshalImages(images),
		normalizeNoteType(noteType), strings.TrimSpace(videoURL), format); err != nil {
		return err
	}
	if s.topics != nil {
		if err := s.topics.ReplaceTopics(ctx, noteID,
			s.topics.ExtractTopics(cursor.StripHTML(normalized), topics)); err != nil {
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
	// 双向拉黑判断：走缓存的 blocked 集合（Redis 30min），不再每请求查库
	blocked, err := s.block.GetBlockedIDs(ctx, currentUserID)
	if err != nil {
		return err
	}
	if containsID(blocked, targetUserID) {
		return ErrBlocked
	}
	blocked, err = s.block.GetBlockedIDs(ctx, targetUserID)
	if err != nil {
		return err
	}
	if containsID(blocked, currentUserID) {
		return ErrBlocked
	}
	return nil
}

func containsID(ids []int64, id int64) bool {
	for _, v := range ids {
		if v == id {
			return true
		}
	}
	return false
}

// 版本列表（只返回 id + 时间，不返回正文，省流量）
func (s *NoteService) ListVersions(ctx context.Context, noteID, cursor, limit int64) ([]*model.NoteVersion, int64, error) {
	return s.repo.ListNoteVersions(ctx, noteID, cursor, int(limit))
}

func (s *NoteService) GetVersion(ctx context.Context, noteID, versionID int64) (*model.NoteVersion, error) {
	return s.repo.GetNoteVersion(ctx, versionID, noteID)
}

// RestoreVersion 恢复某个版本：先快照当前状态（恢复也留痕），再整行写回
func (s *NoteService) RestoreVersion(ctx context.Context, noteID, authorID, versionID int64) error {
	v, err := s.repo.GetNoteVersion(ctx, versionID, noteID)
	if err != nil {
		return ErrVersionNotFound
	}

	// 快照当前状态，保持历史连续
	if current, err := s.GetByID(ctx, noteID); err == nil {
		_ = s.repo.InsertNoteVersion(ctx, &model.NoteVersion{
			NoteID: current.ID, AuthorID: current.AuthorID,
			Title: current.Title, Content: current.Content, Images: current.Images,
			VideoURL: current.VideoURL, Type: current.Type, ContentFormat: current.ContentFormat,
			CreatedAt: time.Now(),
		})
	}

	// 整行写回（含标题/正文/图片/视频/类型/格式）
	if err := s.repo.UpdataByAuthorID(ctx, noteID, authorID,
		v.Title, v.Content, v.Images, v.Type, v.VideoURL, v.ContentFormat); err != nil {
		return err
	}

	// 主题按恢复后的正文重新提取（版本表不存 topics）
	if s.topics != nil {
		_ = s.topics.ReplaceTopics(ctx, noteID,
			s.topics.ExtractTopics(cursor.StripHTML(v.Content), nil))
	}

	// ── 复用 Updata 的失效逻辑：缓存 + 搜索 + feed ──
	_ = s.cache.Delete(ctx, cache.NoteKey(noteID))
	_ = s.cache.Delete(ctx, cache.NoteDetailRawKey(noteID))
	_ = s.cache.Delete(ctx, cache.UserNotesKey(authorID, 10))
	_ = s.cache.Delete(ctx, cache.UserNotesKey(authorID, 20))
	return nil
}
