package service

import (
	"XFeedSystem/internal/cache"
	"XFeedSystem/internal/model"
	"XFeedSystem/internal/repo"
	"context"
	"strconv"
	"strings"
	"time"
)

const (
	statsFlushInterval = 60 * time.Second
	readSetTTL         = 7 * 24 * time.Hour
)

type StatsService struct {
	cache *cache.RedisCache
	repo  repo.StatsRepo
}

func NewStatsService(r repo.StatsRepo, c *cache.RedisCache) *StatsService {
	return &StatsService{repo: r, cache: c}
}

// RecordImpressions 批量记曝光（feed 返回即曝光）
func (s *StatsService) RecordImpressions(ctx context.Context, noteIDs []int64) {
	if s.cache == nil || len(noteIDs) == 0 {
		return
	}
	keys := make([]string, len(noteIDs))
	for i, id := range noteIDs {
		keys[i] = cache.NoteImpKey(id)
	}
	_ = s.cache.IncrMany(ctx, keys, 1)
}

// RecordRead 记阅读（详情打开）
func (s *StatsService) RecordRead(ctx context.Context, noteID int64) {
	if s.cache == nil || noteID <= 0 {
		return
	}
	_, _ = s.cache.IncrBy(ctx, cache.NoteReadKey(noteID), 1)
}

// MarkRead 写入已读集合（登录用户，用于跨会话去重）
func (s *StatsService) MarkRead(ctx context.Context, userID int64, noteIDs []int64) {
	if s.cache == nil || userID <= 0 || len(noteIDs) == 0 {
		return
	}
	key := cache.UserReadKey(userID)
	_ = s.cache.SAdd(ctx, key, noteIDs...)
	_ = s.cache.Expire(ctx, key, readSetTTL)
}

// LoadReadSet 读取用户已读集合
func (s *StatsService) LoadReadSet(ctx context.Context, userID int64) (map[int64]bool, error) {
	out := map[int64]bool{}
	if s.cache == nil || userID <= 0 {
		return out, nil
	}
	ids, err := s.cache.SMembers(ctx, cache.UserReadKey(userID))
	if err != nil {
		return out, err
	}
	for _, id := range ids {
		out[id] = true
	}
	return out, nil
}

// GetStatsMap 拉笔记统计（打分引擎用）
func (s *StatsService) GetStatsMap(ctx context.Context, noteIDs []int64) (map[int64]*model.NoteStats, error) {
	if s.repo == nil {
		return nil, nil
	}
	return s.repo.GetByNoteIDs(ctx, noteIDs)
}

// Flush 把 Redis 计数器落库并清空
func (s *StatsService) Flush(ctx context.Context) error {
	if s.cache == nil || s.repo == nil {
		return nil
	}
	for _, prefix := range []string{"stats:imp:", "stats:read:"} {
		keys, err := s.cache.ScanKeys(ctx, prefix+"*", 500)
		if err != nil {
			return err
		}
		for _, key := range keys {
			val, err := s.cache.Get(ctx, key)
			if err != nil || val == "" {
				continue
			}
			n, err := strconv.ParseInt(val, 10, 64)
			if err != nil {
				continue
			}
			idStr := strings.TrimPrefix(key, prefix)
			noteID, err := strconv.ParseInt(idStr, 10, 64)
			if err != nil || noteID <= 0 {
				continue
			}
			var imp, read int64
			if prefix == "stats:imp:" {
				imp = n
			} else {
				read = n
			}
			_ = s.repo.AddCounters(ctx, noteID, imp, read)
		}
		if len(keys) > 0 {
			_ = s.cache.Delete(ctx, keys...)
		}
	}
	return nil
}

// StartFlusher 启动后台定时落库协程
func (s *StatsService) StartFlusher(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(statsFlushInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				_ = s.Flush(ctx)
			}
		}
	}()
}