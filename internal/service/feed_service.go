package service

import (
	"XFeedSystem/internal/cache"
	"XFeedSystem/internal/model"
	"XFeedSystem/internal/pkg/cursor"
	"XFeedSystem/internal/repo"
	"context"
	"encoding/json"
	"time"
	"sync"
)
var(
	poolLockMu sync.Mutex
	poolLocks = make(map[int64]*sync.Mutex)
)
const(
	feedCacheTTL = 5 * time.Second
	scoredPoolTTL = 10 * time.Second
)

type scoredPool struct {
	GeneratedAt time.Time  `json:"generated_at"`
	Items       []scoredItem `json:"items"`
}

type scoredItem struct {
	ID int64 `json:"id"`
	Score float64 `json:"score"`
}

type FeedListResponse struct {
	Items      []model.FeedItem `json:"items"`
	NextCursor string           `json:"next_cursor"`
}

type FeedService struct {
	repo     *repo.GormFeedRepo
	userRepo *repo.GormUserRepo
	cache    *cache.RedisCache
	search   *repo.SearchRepo
	block    *BlockService
}

func NewFeedService(r *repo.GormFeedRepo, u *repo.GormUserRepo, c *cache.RedisCache, s *repo.SearchRepo, b *BlockService) *FeedService {
	return &FeedService{repo: r, userRepo: u, cache: c, search: s, block: b}
}

func (s *FeedService) ListForYou(ctx context.Context, cursorStr string, limit int, currentUserID int64) (*FeedListResponse, error) {
	// 1. 解析分数游标
	cursorScore, cursorID, err := cursor.ParseScoreCursor(cursorStr)
	if err != nil {
		return nil, err
	}

	// 2. 首页查缓存（key 区分用户）
	if cursorID == 0 && s.cache != nil {
		var resp FeedListResponse
		cacheKey := cache.FeedForYouKeyV2(currentUserID, limit)
		if err := s.cache.GetJSON(ctx, cacheKey, &resp); err == nil {
			return &resp, nil
		}
	}

	// 3. 取打分池（Redis 缓存 10 秒，命中时不再全量拉取+打分+排序）
	pool, err := s.getScoredPool(ctx, currentUserID)
	if err != nil {
		return nil, err
	}

	// 4. 按游标定位起始位置
	startIdx := 0
	if cursorID > 0 {
		for i, it := range pool.Items {
			if it.Score < cursorScore || (it.Score == cursorScore && it.ID < cursorID) {
				startIdx = i + 1
				break
			}
		}
	}

	// 5. 取 limit 条
	endIdx := startIdx + limit
	if endIdx > len(pool.Items) {
		endIdx = len(pool.Items)
	}
	if startIdx >= len(pool.Items) {
		return &FeedListResponse{Items: []model.FeedItem{}, NextCursor: ""}, nil
	}
	page := pool.Items[startIdx:endIdx]

	// 6. 按池内顺序取完整笔记并组装响应
	ids := make([]int64, len(page))
	for i, it := range page {
		ids[i] = it.ID
	}
	notes, err := s.repo.GetByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	byID := make(map[int64]*model.Note, len(notes))
	for _, n := range notes {
		byID[n.ID] = n
	}
	ordered := make([]*model.Note, 0, len(ids))
	for _, id := range ids {
		if n, ok := byID[id]; ok {
			ordered = append(ordered, n)
		}
	}

	resp, err := s.buildFeedResponse(ctx, ordered)
	if err != nil {
		return nil, err
	}

	// 7. 下一页游标 = 本页最后一条的 score + id
	last := page[len(page)-1]
	resp.NextCursor = cursor.EncodeScoreCursor(last.Score, last.ID)

	// 8. 首页写缓存（TTL 与打分池对齐，避免首屏和翻页数据不一致）
	if cursorID == 0 && s.cache != nil {
		_ = s.cache.SetJSON(ctx, cache.FeedForYouKeyV2(currentUserID, limit), resp, scoredPoolTTL)
	}

	return resp, nil
}

func (s *FeedService) buildFeedResponse(ctx context.Context, notes []*model.Note) (*FeedListResponse, error) {
	authorIDs := make([]int64, 0, len(notes))
	seen := make(map[int64]struct{}, len(notes))

	for _, note := range notes {
		if _, ok := seen[note.AuthorID]; ok {
			continue
		}
		seen[note.AuthorID] = struct{}{}
		authorIDs = append(authorIDs, note.AuthorID)
	}
	users := s.batchGetUsers(ctx, authorIDs)
	userMap := make(map[int64]*model.User, len(users))
	for _, u := range users {
		userMap[u.ID] = u
	}

	items := make([]model.FeedItem, 0, len(notes))
	nextCursor := ""

	for _, note := range notes {
		item := model.FeedItem{
			ID:          note.ID,
			AuthorID:    note.AuthorID,
			Title:       note.Title,
			Content:     cursor.BuildSummary(note.Content, 120),
			Images:      parseFeedImages(note.Images),
			Type:        note.Type,
			PublishedAt: note.PublishedAt,
		}

		if u, ok := userMap[note.AuthorID]; ok {
			item.Author = model.AuthorInfo{
				ID:        u.ID,
				Username:  u.Username,
				AvatarURL: u.AvatarURL,
			}
		}

		items = append(items, item)
		nextCursor = cursor.EncodeFeedCursor(note.PublishedAt, note.ID)
	}

	return &FeedListResponse{
		Items:      items,
		NextCursor: nextCursor,
	}, nil
}

func (s *FeedService) getScoredPool(ctx context.Context, currentUserID int64) (*scoredPool, error) {
	key := cache.ScoredPoolKey(currentUserID)
	var pool scoredPool
	if s.cache != nil {
		if err := s.cache.GetJSON(ctx, key, &pool); err == nil && len(pool.Items) > 0 {
			return &pool, nil
		}
	}
	
	lock := poolLockFor(currentUserID)
	lock.Lock()
	defer lock.Unlock()

	if s.cache != nil {
		if err := s.cache.GetJSON(ctx, key, &pool); err == nil && len(pool.Items) > 0 {
			return &pool, nil
		}
	}
	// 拉候选池（ListRecent 已优化为只查必要列）
	notes, err := s.repo.ListRecent(ctx, PoolSize)
	if err != nil {
		return nil, err
	}
	notes = s.filterBlockedNotes(ctx, currentUserID, notes)

	// 准备加权数据（仅登录用户）
	var followingSet map[int64]bool
	var typePref map[int8]float64
	if currentUserID > 0 {
		if ids, err := s.getFollowingIDs(ctx, currentUserID); err == nil {
			followingSet = make(map[int64]bool, len(ids))
			for _, id := range ids {
				followingSet[id] = true
			}
		}
		typePref = s.getTypePref(ctx, currentUserID)
	}

	// 打分排序并写入缓存
	scored := scoreAndSort(notes, time.Now(), followingSet, typePref)
	pool = scoredPool{GeneratedAt: time.Now(), Items: make([]scoredItem, len(scored))}
	for i, sn := range scored {
		pool.Items[i] = scoredItem{ID: sn.Note.ID, Score: sn.Score}
	}
	if s.cache != nil && len(pool.Items) > 0 {
		_ = s.cache.SetJSON(ctx, key, &pool, scoredPoolTTL)
	}
	return &pool, nil
}

func (s *FeedService) ListFollowing(ctx context.Context, userID int64, cursorStr string, limit int) (*FeedListResponse, error) {
	feedCursor, err := cursor.ParseFeedCursor(cursorStr)
	if err != nil {
		return nil, err
	}
	followIDs, err := s.getFollowingIDs(ctx, userID)
	// followIDs, err := s.userRepo.GetFollowingIDs(ctx, userID)
	if err != nil {
		return nil, err
	}

	if len(followIDs) == 0 {
		return &FeedListResponse{
			Items:      []model.FeedItem{},
			NextCursor: "",
		}, nil
	}

	notes, err := s.repo.ListFollowing(ctx, followIDs, feedCursor, limit)
	if err != nil {
		return nil, err
	}
	notes = s.filterBlockedNotes(ctx, userID, notes)
	return s.buildFeedResponse(ctx, notes)
}
func (s *FeedService) getFollowingIDs(ctx context.Context, userID int64) ([]int64, error) {
	key := cache.FollowingIDsKey(userID)
	if s.cache != nil {
		ids, err := s.cache.SMembers(ctx, key)
		if err == nil {
			return ids, nil
		}
		if err != cache.ErrRedisMiss {
		}
	}
	ids, err := s.userRepo.GetFollowingIDs(ctx, userID)
	if err != nil {
		return nil, err
	}
	if s.cache != nil && len(ids) > 0 {
		_ = s.cache.SAdd(ctx, key, ids...)
		_ = s.cache.Expire(ctx, key, 30*time.Minute)
	}
	return ids, nil
}

func (s *FeedService) batchGetUsers(ctx context.Context, ids []int64) []*model.User {
	if len(ids) == 0 {
		return nil
	}

	result := make([]*model.User, 0, len(ids))
	missIDs := make([]int64, 0)

	if s.cache != nil {
		keys := make([]string, len(ids))
		for i, id := range ids {
			keys[i] = cache.UserKey(id)
		}
		vals, err := s.cache.MGet(ctx, keys...)
		if err == nil {
			for i, val := range vals {
				if val == "" {
					missIDs = append(missIDs, ids[i])
					continue
				}
				var u model.User
				if json.Unmarshal([]byte(val), &u) == nil {
					result = append(result, &u)
				} else {
					missIDs = append(missIDs, ids[i])
				}
			}
		} else {
			missIDs = ids
		}
	} else {
		missIDs = ids
	}

	if len(missIDs) > 0 {
		dbUsers, err := s.userRepo.GetByIDs(missIDs)
		if err == nil {
			for _, u := range dbUsers {
				u.PasswordHash = ""
				result = append(result, u)
				if s.cache != nil {
					_ = s.cache.SetJSON(ctx, cache.UserKey(u.ID), u, 1*time.Hour)
				}
			}
		}
	}
	return result
}

func (s *FeedService) SearchNotes(ctx context.Context, keyword string, offset, limit int, currentUserID int64) (*FeedListResponse, int64, error) {
	if s.search == nil {
		return &FeedListResponse{
			Items:      []model.FeedItem{},
			NextCursor: "",
		}, 0, nil
	}

	result, err := s.search.Search(ctx, keyword, offset, limit)
	if err != nil {
		return nil, 0, err
	}

	if len(result.IDs) == 0 {
		return &FeedListResponse{
			Items:      []model.FeedItem{},
			NextCursor: "",
		}, 0, nil
	}

	notes, err := s.repo.GetByIDs(ctx, result.IDs)
	if err != nil {
		return nil, 0, err
	}

	notes = s.filterBlockedNotes(ctx, currentUserID, notes)
	resp, err := s.buildFeedResponse(ctx, notes)
	if err != nil {
		return nil, 0, err
	}
	return resp, result.Total, nil
}

func parseFeedImages(imagesJSON string) []string {
	if imagesJSON == "" {
		return []string{}
	}
	var urls []string
	if err := json.Unmarshal([]byte(imagesJSON), &urls); err != nil {
		return []string{}
	}
	if urls == nil {
		return []string{}
	}
	return urls
}

func (s *FeedService) filterBlockedNotes(ctx context.Context, userID int64, notes []*model.Note) []*model.Note {
	if userID == 0 || s.block == nil {
		return notes
	}
	blockedIDs, err := s.block.GetBlockedIDs(ctx, userID)
	if err != nil || len(blockedIDs) == 0 {
		return notes
	}
	blockedSet := make(map[int64]struct{}, len(blockedIDs))
	for _, id := range blockedIDs {
		blockedSet[id] = struct{}{}
	}
	filtered := notes[:0]
	for _, note := range notes {
		if _, blocked := blockedSet[note.AuthorID]; !blocked {
			filtered = append(filtered, note)
		}
	}
	return filtered
}

func (s *FeedService) getTypePref(ctx context.Context, userID int64) map[int8]float64 {
	key := cache.UserTypePrefKey(userID)
	var pref map[int8]float64
	if s.cache != nil && s.cache.GetJSON(ctx, key, &pref) == nil {
		return pref
	}
	pref, _ = s.repo.GetUserTypePreference(ctx, userID)
	if s.cache != nil {
		_ = s.cache.SetJSON(ctx, key, pref, 10*time.Minute)
	}
	return pref
}

func poolLockFor(userID int64) *sync.Mutex {
	poolLockMu.Lock()
	defer poolLockMu.Unlock()
	if l, ok := poolLocks[userID]; ok {
		return l
	}
	l := &sync.Mutex{}
	poolLocks[userID] = l
	return l
}