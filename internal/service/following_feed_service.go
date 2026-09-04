package service

import (
	"XFeedSystem/internal/cache"
	"XFeedSystem/internal/model"
	"XFeedSystem/internal/pkg/cursor"
	"XFeedSystem/internal/repo"
	"container/heap"
	"context"
	"fmt"
	"strconv"
	"time"
)

// FollowingFeedService Following 读路径：
// User Timeline + Celebrity Author Timeline → K-way Merge → Dedup →
// Follow/Block Filter → Batch Hydration → Cursor Pagination。
//
// 与 For You 的关系：For You 是“先生成候选再排序”，Following 是
// “先物化时间线再合并读取”，因此不复用 For You 的 Ranking 管线。
type FollowingFeedService struct {
	repo     *repo.GormFeedRepo
	userRepo *repo.GormUserRepo
	cache    *cache.RedisCache
	block    *BlockService
	feed     *FeedService // 复用响应组装 / 批量用户查询
}

const (
	// followingFollowerCountTTL 粉丝数缓存 TTL（低频变化）
	followingFollowerCountTTL = 60 * time.Second
)

func NewFollowingFeedService(fr *repo.GormFeedRepo, ur *repo.GormUserRepo, c *cache.RedisCache, b *BlockService, fs *FeedService) *FollowingFeedService {
	return &FollowingFeedService{repo: fr, userRepo: ur, cache: c, block: b, feed: fs}
}

func (s *FollowingFeedService) List(ctx context.Context, userID int64, cursorStr string, limit int) (*FeedListResponse, error) {
	feedCursor, err := cursor.ParseFeedCursor(cursorStr)
	if err != nil {
		return nil, err
	}
	followIDs, err := s.getFollowingIDs(ctx, userID)
	if err != nil {
		return nil, err
	}
	if len(followIDs) == 0 {
		return &FeedListResponse{Items: []model.FeedItem{}, NextCursor: ""}, nil
	}

	hasCursor := feedCursor != nil && !feedCursor.PublishedAt.IsZero()
	cursorMillis, cursorNoteID := int64(0), int64(0)
	if hasCursor {
		cursorMillis = feedCursor.PublishedAt.UnixMilli()
		cursorNoteID = feedCursor.ID
	}

	// Redis Timeline 优先；Redis miss/异常/翻页越过缓存窗口时，MySQL 兜底。
	redisNotes, redisOK, redisErr := s.readRedisTimeline(ctx, userID, followIDs, cursorMillis, cursorNoteID, hasCursor, limit)
	if redisErr != nil {
		redisNotes, redisOK = nil, false // Redis/辅助查询失败 → 降级 MySQL，不让请求失败
	}
	if redisOK && len(redisNotes) >= limit {
		return s.feed.buildFeedResponse(ctx, redisNotes[:limit])
	}

	dbNotes, err := s.repo.ListFollowing(ctx, followIDs, feedCursor, limit*2)
	if err != nil {
		return nil, err
	}
	dbNotes = s.filterNotesByFollowAndBlock(ctx, userID, followIDs, dbNotes)

	combined := mergeNoteLists(redisNotes, dbNotes)
	if len(combined) == 0 {
		return &FeedListResponse{Items: []model.FeedItem{}, NextCursor: ""}, nil
	}
	if len(combined) > limit {
		combined = combined[:limit]
	}
	if !redisOK {
		// 冷启动：本次先由 MySQL 保证正确性，后台异步把 Timeline materialize 起来
		s.materializeTimelineAsync(userID, followIDs)
	}
	return s.feed.buildFeedResponse(ctx, combined)
}

func (s *FollowingFeedService) getFollowingIDs(ctx context.Context, userID int64) ([]int64, error) {
	return s.feed.getFollowingIDs(ctx, userID)
}

// readRedisTimeline 从 Redis 读取并返回按 (published_at, noteID) 倒序的水合结果。
// ok=false 表示走 MySQL 兜底（key 不存在 / 空 / 超出缓存窗口）。
func (s *FollowingFeedService) readRedisTimeline(ctx context.Context, userID int64, followIDs []int64, cursorMillis int64, cursorNoteID int64, hasCursor bool, limit int) ([]*model.Note, bool, error) {
	if s.cache == nil {
		return nil, false, nil
	}
	userKey := cache.FollowingTimelineKey(userID)
	zc, err := s.cache.ZCard(ctx, userKey)
	if err != nil || zc == 0 {
		return nil, false, nil // 冷启动：交给 MySQL fallback
	}

	// Celebrity 判定：follower_count >= threshold 的作者走 Pull（带 60s 缓存）
	counts, err := s.getFollowerCounts(ctx, followIDs)
	if err != nil {
		return nil, false, err
	}

	need := limit * 2 // 过滤会吞掉一部分候选，先多取一些再裁
	sources := make([]*timelineMergeSource, 0, len(followIDs)+1)
	sources = append(sources, newTimelineMergeSource(
		s.cache, userKey, cursorMillis, cursorNoteID, hasCursor, need,
	))
	for _, fid := range followIDs {
		if counts[fid] < FollowingFanoutThreshold {
			continue
		}
		sources = append(sources, newTimelineMergeSource(
			s.cache, cache.AuthorTimelineKey(fid), cursorMillis, cursorNoteID, hasCursor, need,
		))
	}

	merged, err := mergeTimelineItems(ctx, sources, need)
	if err != nil {
		return nil, true, err
	}
	if len(merged) == 0 {
		return []*model.Note{}, true, nil
	}

	ids := make([]int64, 0, len(merged))
	for _, it := range merged {
		ids = append(ids, it.NoteID)
	}
	hydrated := s.hydrateAndFilterNotes(ctx, userID, followIDs, ids)
	if len(hydrated) == 0 {
		return []*model.Note{}, true, nil
	}
	return hydrated, true, nil
}

// filterCandidateIDs Redis 候选只按 ID 排好序，还没碰过 MySQL：
// 先批量查出 author_id，做 follow（Unfollow 竞态）与 block 过滤，再做水合。
func (s *FollowingFeedService) filterCandidateIDs(ctx context.Context, userID int64, followIDs []int64, noteIDs []int64) ([]int64, error) {
	if len(noteIDs) == 0 {
		return nil, nil
	}
	authorByNote, err := s.repo.GetNoteAuthorIDs(ctx, noteIDs)
	if err != nil {
		return nil, err
	}
	followSet := make(map[int64]struct{}, len(followIDs))
	for _, id := range followIDs {
		followSet[id] = struct{}{}
	}

	var blockedSet map[int64]struct{}
	if s.block != nil {
		blocked, err := s.block.GetBlockedIDs(ctx, userID)
		if err == nil && len(blocked) > 0 {
			blockedSet = make(map[int64]struct{}, len(blocked))
			for _, id := range blocked {
				blockedSet[id] = struct{}{}
			}
		}
	}

	keep := make([]int64, 0, len(noteIDs))
	for _, noteID := range noteIDs {
		authorID, ok := authorByNote[noteID]
		if !ok {
			continue // 已被硬删除：直接丢
		}
		if _, ok := followSet[authorID]; !ok {
			continue // Unfollow 竞态 / stale fanout：读时校验
		}
		if _, bad := blockedSet[authorID]; bad {
			continue
		}
		keep = append(keep, noteID)
	}
	return keep, nil
}

// filterNotesByFollowAndBlock MySQL fallback 结果的 Follow/Block 读时校验
func (s *FollowingFeedService) filterNotesByFollowAndBlock(ctx context.Context, userID int64, followIDs []int64, notes []*model.Note) []*model.Note {
	if len(notes) == 0 {
		return notes
	}
	followSet := make(map[int64]struct{}, len(followIDs))
	for _, id := range followIDs {
		followSet[id] = struct{}{}
	}
	var blockedSet map[int64]struct{}
	if s.block != nil {
		blocked, err := s.block.GetBlockedIDs(ctx, userID)
		if err == nil && len(blocked) > 0 {
			blockedSet = make(map[int64]struct{}, len(blocked))
			for _, id := range blocked {
				blockedSet[id] = struct{}{}
			}
		}
	}
	kept := notes[:0]
	for _, n := range notes {
		if _, ok := followSet[n.AuthorID]; !ok {
			continue
		}
		if _, bad := blockedSet[n.AuthorID]; bad {
			continue
		}
		kept = append(kept, n)
	}
	return kept
}

// hydrateNotes Batch Hydration：一条 IN 查询 + 按 Redis 顺序恢复
func (s *FollowingFeedService) hydrateNotes(ctx context.Context, ids []int64) []*model.Note {
	if len(ids) == 0 {
		return nil
	}
	notes, err := s.repo.GetByIDs(ctx, ids)
	if err != nil {
		return nil // Redis 只负责候选，MySQL 失败时本轮返回空，交给下一轮/MySQL fallback
	}
	byID := make(map[int64]*model.Note, len(notes))
	for _, n := range notes {
		byID[n.ID] = n
	}
	out := make([]*model.Note, 0, len(ids))
	for _, id := range ids {
		if n, ok := byID[id]; ok {
			out = append(out, n)
		}
	}
	return out
}

// mergeNoteLists 合并“Redis 已水合结果”与“MySQL fallback 结果”。
// 两边都已经按 (published_at, noteID) 倒序，做稳定归并去重即可。
func mergeNoteLists(a, b []*model.Note) []*model.Note {
	if len(a) == 0 {
		return b
	}
	if len(b) == 0 {
		return a
	}
	out := make([]*model.Note, 0, len(a)+len(b))
	seen := make(map[int64]struct{}, len(a)+len(b))
	i, j := 0, 0
	for i < len(a) || j < len(b) {
		var pick *model.Note
		if j >= len(b) || (i < len(a) && noteNewer(a[i], b[j])) {
			pick = a[i]
			i++
		} else {
			pick = b[j]
			j++
		}
		if _, dup := seen[pick.ID]; dup {
			continue
		}
		seen[pick.ID] = struct{}{}
		out = append(out, pick)
	}
	return out
}

func noteNewer(a, b *model.Note) bool {
	if a.PublishedAt.Equal(b.PublishedAt) {
		return a.ID > b.ID
	}
	return a.PublishedAt.After(b.PublishedAt)
}

// materializeTimelineAsync 冷启动回填：MySQL 返回正确结果后，后台把
// 每个已关注作者最近 N 条写进 feed:following:{uid}，让后续请求走 Redis。
func (s *FollowingFeedService) materializeTimelineAsync(userID int64, followIDs []int64) {
	if s.cache == nil || len(followIDs) == 0 {
		return
	}
	lockKey := fmt.Sprintf("feed:following:materialize:%d", userID)
	lockCtx, cancelLock := context.WithTimeout(context.Background(), 2*time.Second)
	ok, err := s.cache.SetNX(lockCtx, lockKey, "1", time.Minute)
	cancelLock()
	if err != nil || !ok {
		return // 已有请求在 materialize，避免惊群
	}
	safeGo(func() {
		defer func() {
			delCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			_ = s.cache.Delete(delCtx, lockKey)
		}()
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		key := cache.FollowingTimelineKey(userID)
		for _, authorID := range followIDs {
			if ctx.Err() != nil {
				return
			}
			notes, err := s.repo.ListRecentByAuthor(ctx, authorID, FollowingBackfillLimit)
			if err != nil || len(notes) == 0 {
				continue
			}
			adds := make([]cache.TimelineAdd, 0, len(notes))
			for i, n := range notes {
				a := cache.TimelineAdd{
					Key:    key,
					NoteID: n.ID,
					Millis: n.PublishedAt.UnixMilli(),
				}
				if i == len(notes)-1 {
					a.Trim = FollowingTimelineSize
				}
				adds = append(adds, a)
			}
			_ = s.cache.ZAddTimelineBatch(ctx, adds)
		}
	})
}

func (s *FollowingFeedService) getFollowerCounts(ctx context.Context, followIDs []int64) (map[int64]int64, error) {
	out := make(map[int64]int64, len(followIDs))
	if len(followIDs) == 0 || s.cache == nil {
		return s.repo.GetFollowerCounts(ctx, followIDs)
	}
	keys := make([]string, 0, len(followIDs))
	for _, id := range followIDs {
		keys = append(keys, cache.FollowingCountKey(id))
	}
	missing := make([]int64, 0, len(followIDs))
	vals, err := s.cache.MGet(ctx, keys...)
	if err != nil {
		missing = append(missing, followIDs...)
	} else {
		for i, v := range vals {
			if v == "" {
				missing = append(missing, followIDs[i])
				continue
			}
			if n, err := strconv.ParseInt(v, 10, 64); err == nil {
				out[followIDs[i]] = n
			} else {
				missing = append(missing, followIDs[i])
			}
		}
	}
	if len(missing) > 0 {
		dbCounts, err := s.repo.GetFollowerCounts(ctx, missing)
		if err != nil {
			if len(out) == 0 {
				return nil, err
			}
			return out, nil // 部分命中可用，不阻塞
		}
		for id, c := range dbCounts {
			out[id] = c
			_ = s.cache.Set(ctx, cache.FollowingCountKey(id), strconv.FormatInt(c, 10), followingFollowerCountTTL)
		}
	}
	return out, nil
}

func (s *FollowingFeedService) hydrateAndFilterNotes(ctx context.Context, userID int64, followIDs []int64, noteIDs []int64) []*model.Note {
	if len(noteIDs) == 0 || s.cache == nil {
		return nil
	}
	notes, err := s.repo.GetByIDs(ctx, noteIDs)
	if err != nil || len(notes) == 0 {
		return nil
	}
	byID := make(map[int64]*model.Note, len(notes))
	for _, n := range notes {
		byID[n.ID] = n
	}
	followSet := make(map[int64]struct{}, len(followIDs))
	for _, id := range followIDs {
		followSet[id] = struct{}{}
	}
	var blockedSet map[int64]struct{}
	if s.block != nil {
		if blocked, err := s.block.GetBlockedIDs(ctx, userID); err == nil && len(blocked) > 0 {
			blockedSet = make(map[int64]struct{}, len(blocked))
			for _, id := range blocked {
				blockedSet[id] = struct{}{}
			}
		}
	}
	out := make([]*model.Note, 0, len(noteIDs))
	for _, id := range noteIDs {
		n := byID[id]
		if n == nil {
			continue // 已删除/未发布
		}
		if _, ok := followSet[n.AuthorID]; !ok {
			continue // Unfollow 竞态 / stale fanout
		}
		if _, bad := blockedSet[n.AuthorID]; bad {
			continue
		}
		out = append(out, n)
	}
	return out
}

// ---------- K-way Merge（container/heap 大顶堆） ----------

type timelineMergeSource struct {
	cache        *cache.RedisCache
	key          string
	pageSize     int
	hasCursor    bool
	cursorMillis int64
	cursorNoteID int64
	buf          []cache.TimelineItem
	idx          int
	started      bool
	exhausted    bool
}

func newTimelineMergeSource(c *cache.RedisCache, key string, cursorMillis int64, cursorNoteID int64, hasCursor bool, pageSize int) *timelineMergeSource {
	return &timelineMergeSource{
		cache:        c,
		key:          key,
		pageSize:     pageSize,
		hasCursor:    hasCursor,
		cursorMillis: cursorMillis,
		cursorNoteID: cursorNoteID,
	}
}

// next 每次返回该有序流的下一条；内部按 pageSize 缓冲，
// 缓冲耗尽后以“本条流最后一条”为游标再取下一页。
func (src *timelineMergeSource) next(ctx context.Context) (cache.TimelineItem, bool, error) {
	if src.exhausted {
		return cache.TimelineItem{}, false, nil
	}
	if src.idx >= len(src.buf) {
		if src.started {
			src.hasCursor = true
		}
		items, err := src.cache.ListTimelinePage(
			ctx, src.key, src.cursorMillis, src.cursorNoteID, src.hasCursor, src.pageSize,
		)
		if err != nil {
			return cache.TimelineItem{}, false, err
		}
		src.started = true
		if len(items) == 0 {
			src.exhausted = true
			return cache.TimelineItem{}, false, nil
		}
		last := items[len(items)-1]
		src.cursorMillis = last.Millis
		src.cursorNoteID = last.NoteID
		src.buf = items
		src.idx = 0
	}
	item := src.buf[src.idx]
	src.idx++
	return item, true, nil
}

type timelineHeapItem struct {
	item cache.TimelineItem
	src  *timelineMergeSource
}

// followingItemHeap 按 (score desc, noteID desc) 的大顶堆
type followingItemHeap []timelineHeapItem

func (h followingItemHeap) Len() int      { return len(h) }
func (h followingItemHeap) Swap(i, j int) { h[i], h[j] = h[j], h[i] }
func (h followingItemHeap) Less(i, j int) bool {
	if h[i].item.Millis != h[j].item.Millis {
		return h[i].item.Millis > h[j].item.Millis
	}
	return h[i].item.NoteID > h[j].item.NoteID
}
func (h *followingItemHeap) Push(x interface{}) { *h = append(*h, x.(timelineHeapItem)) }
func (h *followingItemHeap) Pop() interface{} {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[:n-1]
	return x
}

// mergeTimelineItems 把 K 条有序流归并成一条全局有序、去重的候选序列
func mergeTimelineItems(ctx context.Context, sources []*timelineMergeSource, need int) ([]cache.TimelineItem, error) {
	if need <= 0 || len(sources) == 0 {
		return nil, nil
	}
	h := &followingItemHeap{}
	for _, src := range sources {
		item, ok, err := src.next(ctx)
		if err != nil {
			return nil, err
		}
		if ok {
			heap.Push(h, timelineHeapItem{item: item, src: src})
		}
	}
	out := make([]cache.TimelineItem, 0, need)
	seen := make(map[int64]struct{}, need)
	for h.Len() > 0 && len(out) < need {
		top := heap.Pop(h).(timelineHeapItem)
		if _, dup := seen[top.item.NoteID]; !dup {
			seen[top.item.NoteID] = struct{}{}
			out = append(out, top.item)
		}
		item, ok, err := top.src.next(ctx)
		if err != nil {
			return nil, err
		}
		if ok {
			heap.Push(h, timelineHeapItem{item: item, src: top.src})
		}
	}
	return out, nil
}
