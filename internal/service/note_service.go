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
	repo     repo.NoteRepo
	cache    *cache.RedisCache
	search   *repo.SearchRepo
	notifSvc *NotificationService
	block    *BlockService
	topics   *TopicService
	feed     *FeedService
	user     repo.UserRepo
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

func NewNoteService(r repo.NoteRepo, c *cache.RedisCache, sr *repo.SearchRepo,
	ns *NotificationService, b *BlockService, t *TopicService, f *FeedService, us repo.UserRepo) *NoteService {
	return &NoteService{repo: r, cache: c, search: sr, notifSvc: ns, block: b, topics: t, feed: f, user: us}
}

// invalidateNoteFeed 笔记级写操作后的缓存失效：
//   - 同步删笔记 JSON + 详情字节缓存（精确 key，立刻生效）
//   - 异步失效全量打分引擎 + feed 页字节缓存（需 SCAN，不阻塞写响应）
func (s *NoteService) invalidateNoteFeed(ctx context.Context, noteID int64) {
	if s.cache != nil {
		_ = s.cache.Delete(ctx, cache.NoteKey(noteID), cache.NoteDetailRawKey(noteID))
		_ = s.cache.InvalidateFeedRawAll(ctx) // 页字节缓存仍要删
	}
	if s.feed != nil {
		_ = s.feed.UpsertNoteScore(ctx, noteID) // 单条 ZADD，替代 InvalidateFeedEngineAll
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
	if _, err := s.repo.Create(note); err != nil {
		return nil, err
	}

	// 话题识别用纯文本，避免匹配到 HTML 属性里的 #xx
	if s.topics != nil {
		if err := s.topics.AttachToNote(context.Background(), note.ID,
			s.topics.ExtractTopics(cursor.StripHTML(normalized), topics)); err != nil {
			logger.Sugar.Errorf("attach topics err: %v", err)
		}
	}
	if s.notifSvc != nil {
		s.notifyMentions(context.Background(), userID, note.ID, note.ID,
			cursor.StripHTML(normalized), nil)
	}
	if s.cache != nil {
		_ = s.cache.Delete(context.Background(), cache.NoteKey(note.ID), cache.NoteDetailRawKey(note.ID))
		_ = s.cache.Delete(context.Background(),
			cache.UserNotesKey(note.AuthorID, 10),
			cache.UserNotesKey(note.AuthorID, 20),
		)
	}
	safeGo(func() {
		if s.feed != nil {
			// 增量 ZADD：新笔记直接进打分 ZSET，避免全量删除与后续写操作竞态
			_ = s.feed.UpsertNoteScore(context.Background(), note.ID)
		}
		_ = s.cache.InvalidateFeedRawAll(context.Background())
	})

	if s.search != nil {
		n := note
		safeGo(func() {
			_ = s.search.Index(context.Background(), &repo.NoteDocument{
				ID:          n.ID,
				AuthorID:    n.AuthorID,
				Title:       n.Title,
				Content:     cursor.StripHTML(n.Content),
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
	// 从打分 ZSET 移除，避免已删笔记残留占位
	if s.feed != nil {
		_ = s.feed.RemoveNoteScore(context.Background(), id)
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
			// 一级评论：通知笔记作者（类型 2）+ @提及
			safeGo(func() {
				s.notifSvc.Create(context.Background(), userID, n.AuthorID,
					model.NotifTypeComment, comment.ID, noteID, "评论了你的笔记")
			})
			s.notifyMentions(context.Background(), userID, comment.ID, noteID, content,
				map[int64]bool{n.AuthorID: true})
		} else {
			skip := map[int64]bool{userID: true}
			// 被回复的评论作者 → 类型 7"回复了你的评论"
			skip[replyToUserID] = true
			safeGo(func() {
				s.notifSvc.Create(context.Background(), userID, replyToUserID,
					model.NotifTypeReplyComment, comment.ID, noteID, "回复了你的评论")
			})
			// 笔记作者（若不是被回复者）→ 类型 2"评论了你的笔记"，避免同一条回复打扰两次
			if replyToUserID != n.AuthorID {
				skip[n.AuthorID] = true
				safeGo(func() {
					s.notifSvc.Create(context.Background(), userID, n.AuthorID,
						model.NotifTypeComment, comment.ID, noteID, "评论了你的笔记")
				})
			}
			// 回复内容里的 @提及
			s.notifyMentions(context.Background(), userID, comment.ID, noteID, content, skip)
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
	if old, err := s.repo.GetByID(ctx, noteID); err == nil && old.AuthorID == authorID {
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
	if s.search != nil {
		safeGo(func() {
			_, err := s.repo.GetByID(ctx, noteID)
			if err != nil {
				return
			}
			_ = s.search.Index(context.Background(), &repo.NoteDocument{
				ID:          noteID,
				Title:       title,
				Content:     cursor.StripHTML(normalized),
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

func (s *NoteService) notifyMentions(ctx context.Context, actorID, targetID, noteID int64, content string, extraSkip map[int64]bool) {
	names := extractMentions(content)
	if len(names) == 0 || s.notifSvc == nil {
		return
	}
	users, err := s.user.FindByUsernames(ctx, names)
	if err != nil || len(users) == 0 {
		return
	}
	for _, u := range users {
		if u.ID == actorID || (extraSkip != nil && extraSkip[u.ID]) {
			continue
		}
		if s.block != nil {
			if blocked, err := s.block.IsBlockedEitherWay(ctx, actorID, u.ID); err == nil && blocked {
				continue
			}
		}
		s.notifSvc.Create(ctx, actorID, u.ID, model.NotifTypeMention, targetID, noteID, "@提到了你")
	}
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
	if current, err := s.repo.GetByID(ctx, noteID); err == nil {
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
	if s.search != nil {
		safeGo(func() {
			_ = s.search.Index(context.Background(), &repo.NoteDocument{
				ID: noteID, Title: v.Title, Content: cursor.StripHTML(v.Content),
				AuthorID: authorID, PublishedAt: time.Now().Unix(),
			})
		})
	}
	if s.feed != nil {
		_ = s.feed.UpsertNoteScore(ctx, noteID) // 恢复改变了摘要，刷新打分
	}
	_ = s.cache.InvalidateFeedRawAll(ctx)
	return nil
}
