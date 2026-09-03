package service

import (
	"XFeedSystem/internal/cache"
	"XFeedSystem/internal/events"
	"XFeedSystem/internal/model"
	"XFeedSystem/internal/repo"
	"context"
	"errors"

	"gorm.io/gorm"
)

// Following 配置集中在同一处，避免阈值/容量散落在代码里。
const (
	// FollowingFanoutThreshold 粉丝数达到该值走 Celebrity Pull（Fan-out on Read）
	FollowingFanoutThreshold = 10_000
	// FollowingTimelineSize Redis Timeline 保留的最近条数
	FollowingTimelineSize = 800
	// FollowingFanoutBatchSize 每个 Redis Pipeline 的粉丝批大小
	FollowingFanoutBatchSize = 500
	// FollowingBackfillLimit Follow 回填 / 冷启动 materialize 拉取每个作者的最近条数
	FollowingBackfillLimit = 100
)

// FollowingFanoutService 写路径：NoteCreated Fanout / NoteDeleted 清理 /
// UserFollowed Backfill / UserUnfollowed Cleanup。
// 事件由 GroupFeed Consumer 驱动，HTTP 请求不等待 fanout。
type FollowingFanoutService struct {
	feedRepo *repo.GormFeedRepo
	userRepo *repo.GormUserRepo
	noteRepo *repo.GormNoteRepo
	cache    *cache.RedisCache
}

func NewFollowingFanoutService(
	fr *repo.GormFeedRepo,
	ur *repo.GormUserRepo,
	nr *repo.GormNoteRepo,
	c *cache.RedisCache,
) *FollowingFanoutService {
	return &FollowingFanoutService{feedRepo: fr, userRepo: ur, noteRepo: nr, cache: c}
}

// HandleNoteCreated 发布流程：
// 1) 无论作者规模，先把 NoteID 写进 feed:author:{authorID}（Celebrity Pull 数据源）；
// 2) 粉丝数 < 阈值 → Fan-out on Write，分页推送；
// 3) 粉丝数 >= 阈值 → 只保留 Author Timeline，由读路径合并。
func (s *FollowingFanoutService) HandleNoteCreated(ctx context.Context, p events.Payload) error {
	if s.cache == nil {
		return nil
	}
	note, err := s.noteRepo.GetByID(ctx, p.NoteID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil // 笔记不存在，事件无需处理
		}
		return err
	}
	if note.Status != model.NoteStatusPublished {
		return nil
	}

	// 作者时间线：NoteID + published_at（Redis 只存 ID，正文仍以 MySQL 为准）
	if err := s.cache.ZAddTimelineBatch(ctx, []cache.TimelineAdd{{
		Key:    cache.AuthorTimelineKey(note.AuthorID),
		NoteID: note.ID,
		Millis: note.PublishedAt.UnixMilli(),
		Trim:   FollowingTimelineSize,
	}}); err != nil {
		return err
	}

	count, err := s.userRepo.CountFollowers(ctx, note.AuthorID)
	if err != nil {
		return err
	}
	if count >= FollowingFanoutThreshold {
		return nil // Celebrity：不推给粉丝，读路径 Pull
	}
	return s.pushNoteToFollowers(ctx, note)
}

// pushNoteToFollowers Fan-out on Write：
// 粉丝按 user_id 键集分页，每批 500 条走一个 Redis Pipeline，避免超大请求。
func (s *FollowingFanoutService) pushNoteToFollowers(ctx context.Context, note *model.Note) error {
	afterUserID := int64(0)
	score := note.PublishedAt.UnixMilli()
	for {
		ids, err := s.userRepo.ListFollowerIDsPage(ctx, note.AuthorID, afterUserID, FollowingFanoutBatchSize)
		if err != nil {
			return err
		}
		if len(ids) == 0 {
			return nil
		}

		adds := make([]cache.TimelineAdd, 0, len(ids))
		for _, followerID := range ids {
			adds = append(adds, cache.TimelineAdd{
				Key:    cache.FollowingTimelineKey(followerID),
				NoteID: note.ID,
				Millis: score,
				Trim:   FollowingTimelineSize,
			})
			afterUserID = followerID
		}
		if err := s.cache.ZAddTimelineBatch(ctx, adds); err != nil {
			return err
		}
		if len(ids) < FollowingFanoutBatchSize {
			return nil
		}
	}
}

// HandleNoteDeleted 只同步删除 Author Timeline；用户 Timeline 采用“读时校验 + 惰性清理”。
// 原因：删除时遍历百万粉丝做 ZREM 会把写路径拖垮，Redis 短暂保留 stale ID 是可接受的，
// 读路径 Batch Hydration 只会查出 status=published 的笔记。
func (s *FollowingFanoutService) HandleNoteDeleted(ctx context.Context, p events.Payload) error {
	if s.cache == nil {
		return nil
	}
	return s.cache.ZRemTimeline(ctx, cache.AuthorTimelineKey(p.AuthorID), p.NoteID)
}

// HandleNoteUpdated Following Timeline 不依赖 title/content，通常无需处理。
// 只有 published_at 变化才需要重新打分；当前 Update 不允许改 published_at，因此空实现。
func (s *FollowingFanoutService) HandleNoteUpdated(ctx context.Context, p events.Payload) error {
	return nil
}

// HandleUserFollowed Follow Backfill：把被关注作者最近 N 条写进关注者 Timeline，
// 让 Follow 成功后能立刻看到对方的近期内容（不等下一次发布）。
func (s *FollowingFanoutService) HandleUserFollowed(ctx context.Context, p events.Payload) error {
	if s.cache == nil {
		return nil
	}
	notes, err := s.feedRepo.ListRecentByAuthor(ctx, p.AuthorID, FollowingBackfillLimit)
	if err != nil {
		return err
	}
	if len(notes) == 0 {
		return nil
	}

	key := cache.FollowingTimelineKey(p.ActorID)
	adds := make([]cache.TimelineAdd, 0, len(notes))
	for i, n := range notes {
		a := cache.TimelineAdd{
			Key:    key,
			NoteID: n.ID,
			Millis: n.PublishedAt.UnixMilli(),
		}
		if i == len(notes)-1 {
			a.Trim = FollowingTimelineSize // 同一 key 只裁剪一次
		}
		adds = append(adds, a)
	}
	return s.cache.ZAddTimelineBatch(ctx, adds)
}

// HandleUserUnfollowed 异步清理：
// 旧 fanout Job 可能晚到，因此读路径还会做 follow 关系校验；这里做的是尽力清理。
func (s *FollowingFanoutService) HandleUserUnfollowed(ctx context.Context, p events.Payload) error {
	if s.cache == nil {
		return nil
	}
	key := cache.FollowingTimelineKey(p.ActorID)
	items, err := s.cache.ListTimelinePage(ctx, key, 0, 0, false, int(FollowingTimelineSize))
	if err != nil {
		return err
	}
	if len(items) == 0 {
		return nil
	}

	ids := make([]int64, 0, len(items))
	for _, it := range items {
		ids = append(ids, it.NoteID)
	}
	authorByNote, err := s.feedRepo.GetNoteAuthorIDs(ctx, ids)
	if err != nil {
		return err
	}

	var remove []int64
	for _, it := range items {
		if authorByNote[it.NoteID] == p.AuthorID {
			remove = append(remove, it.NoteID)
		}
	}
	return s.cache.ZRemTimeline(ctx, key, remove...)
}
