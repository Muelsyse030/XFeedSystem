package service

import (
	"XFeedSystem/internal/cache"
	"XFeedSystem/internal/model"
	"XFeedSystem/internal/pkg/cursor"
	"XFeedSystem/internal/repo"
	"context"
	"encoding/json"
	"math"
	"sort"
	"strconv"
	"sync"
	"time"
)

const (
	feedEngineTTL = 60 * time.Second // 打分 ZSET 重建周期
	feedScoreMult = 1_000_000        // 分数折叠因子：zsetScore = round(score*10000)*mult + id，消除同分并列
)

// scoredFeedItem 打分页元素
type scoredFeedItem struct {
	ID       int64
	Score    float64
	BaseScore float64
	AuthorID int64
}

var (
	engineLockMu sync.Mutex
	engineLocks  = make(map[int64]*sync.Mutex)
)

func engineLockFor(userID int64) *sync.Mutex {
	engineLockMu.Lock()
	defer engineLockMu.Unlock()
	if l, ok := engineLocks[userID]; ok {
		return l
	}
	l := &sync.Mutex{}
	engineLocks[userID] = l
	return l
}

// foldScore 把 (分数, id) 折叠成唯一整数分数：score*10000*mult + id，ZSET 内严格有序无并列
func foldScore(score float64, id int64) int64 {
	return int64(math.Round(score*10000))*feedScoreMult + id
}

// unfoldScore 从折叠分数还原 (4位小数分数, id)
func unfoldScore(z int64) (float64, int64) {
	return float64(z/feedScoreMult) / 10000.0, z % feedScoreMult
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
	stats	 *StatsService
}

func NewFeedService(r *repo.GormFeedRepo, u *repo.GormUserRepo, c *cache.RedisCache, s *repo.SearchRepo, b *BlockService , st *StatsService) *FeedService {
	return &FeedService{repo: r, userRepo: u, cache: c, search: s, block: b , stats : st}
}

func (s *FeedService) ListForYou(ctx context.Context, cursorStr string, limit int, currentUserID int64) (*FeedListResponse, error) {
	// Feed 引擎：Redis ZSET 预计算打分，全部笔记参与排序，游标分页 O(log n)
	cursorScore, cursorID, err := cursor.ParseScoreCursor(cursorStr)
	if err != nil {
		return nil, err
	}

	page, start, err := s.getFeedPage(ctx, currentUserID, cursorScore, cursorID, limit)
	if err != nil {
		return nil, err
	}
	if len(page) == 0 {
		return &FeedListResponse{Items: []model.FeedItem{}, NextCursor: ""}, nil
	}

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
	ordered = s.filterBlockedNotes(ctx, currentUserID, ordered)
	if len(ordered) == 0 {
		return &FeedListResponse{Items: []model.FeedItem{}, NextCursor: ""}, nil
	}

	resp, err := s.buildFeedResponse(ctx, ordered)
	if err != nil {
		return nil, err
	}
	if s.stats != nil {
		shownIDs := make([]int64, 0, len(ordered))
		for _, n := range ordered {
			shownIDs = append(shownIDs, n.ID)
		}
		s.stats.RecordImpressions(ctx, shownIDs)
	}
	// 下一页游标 = 个性化排序后的位置偏移（顺序稳定，不重不漏）
	if int64(len(page)) == int64(limit) {
		resp.NextCursor = cursor.EncodeScoreCursor(float64(start+int64(len(page))), 0)
	} else {
		resp.NextCursor = ""
	}
	return resp, nil
}

// ListTopic 话题页 feed（时间倒序键集分页，复用响应组装）
func (s *FeedService) ListTopic(ctx context.Context, topicID int64, cursorStr string, limit int, currentUserID int64) (*FeedListResponse, error) {
	feedCursor, err := cursor.ParseFeedCursor(cursorStr)
	if err != nil {
		return nil, err
	}
	notes, err := s.repo.ListByTopic(ctx, topicID, feedCursor, limit)
	if err != nil {
		return nil, err
	}
	notes = s.filterBlockedNotes(ctx, currentUserID, notes)
	return s.buildFeedResponse(ctx, notes)
}

// getFeedPage 全量候选 + 读时个性化排序后，按位置偏移分页。
// 为什么不用"基础分窗口 + 基础分游标"：个性化排序和基础分顺序不一致时，
// 基础分游标会永久跳过"基础分靠前但个性化排后"的笔记（漏内容）。
// 当前站点规模小（百级笔记），全量取 ZSET 再排序成本极低，换来不重不漏。
func (s *FeedService) getFeedPage(ctx context.Context, userID int64, cursorScore float64, cursorID int64, limit int) ([]scoredFeedItem, int64, error) {
	key := cache.FeedEngineKey(0)
	if err := s.ensureFeedEngine(ctx, key); err != nil {
		return nil, 0, err
	}

	start := int64(0)
	if cursorID == 0 && cursorScore > 0 {
		start = int64(cursorScore)
	}

	// 读时个性化参数
	var followingSet map[int64]bool
	var typePref map[int8]float64
	if userID > 0 {
		if ids, err := s.getFollowingIDs(ctx, userID); err == nil {
			followingSet = make(map[int64]bool, len(ids))
			for _, id := range ids {
				followingSet[id] = true
			}
		}
		typePref, _ = s.repo.GetUserTypePreference(ctx, userID)
	}

	// 全量候选：按基础分倒序取全部
	zs, err := s.cache.ZRevRangeByScore(ctx, key, "+inf", "-inf", 0, -1)
	if err != nil {
		return nil, 0, err
	}
	ids := make([]int64, 0, len(zs))
	candidates := make([]scoredFeedItem, 0, len(zs))
	for _, z := range zs {
		member, ok := z.Member.(string)
		if !ok {
			continue
		}
		id, err := strconv.ParseInt(member, 10, 64)
		if err != nil {
			continue
		}
		score, _ := unfoldScore(int64(z.Score))
		ids = append(ids, id)
		candidates = append(candidates, scoredFeedItem{ID: id, Score: score, BaseScore: score})
	}
	if len(candidates) == 0 {
		return []scoredFeedItem{}, start, nil
	}

	authorByNote, _ := s.repo.GetNoteAuthorIDs(ctx, ids)
	typeByNote, _ := s.repo.GetNoteTypes(ctx, ids)
	for i := range candidates {
		it := &candidates[i]
		authorID := authorByNote[it.ID]
		it.Score = personalizedScore(it.Score, authorID, typeByNote[it.ID], followingSet, typePref)
		it.AuthorID = authorID
	}

	// 个性化排序（分数降序，同分 id 降序）
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].Score != candidates[j].Score {
			return candidates[i].Score > candidates[j].Score
		}
		return candidates[i].ID > candidates[j].ID
	})

	// 拉黑过滤：按作者去重后一次性查询
	blockedAuthors := make(map[int64]bool)
	if userID > 0 && s.block != nil {
		authorSet := make(map[int64]struct{}, len(candidates))
		for _, it := range candidates {
			authorSet[it.AuthorID] = struct{}{}
		}
		for aid := range authorSet {
			if blocked, err := s.block.IsBlockedEitherWay(ctx, userID, aid); err == nil && blocked {
				blockedAuthors[aid] = true
			}
		}
	}

	// 作者去重（自己的笔记不限量）+ 位置分页
	const maxPerAuthor = 2
	authorCount := map[int64]int{}
	full := make([]scoredFeedItem, 0, len(candidates))
	for _, it := range candidates {
		if blockedAuthors[it.AuthorID] {
			continue
		}
		if it.AuthorID != userID && authorCount[it.AuthorID] >= maxPerAuthor {
			continue
		}
		full = append(full, it)
		authorCount[it.AuthorID]++
	}

	if start >= int64(len(full)) {
		return []scoredFeedItem{}, start, nil
	}
	end := start + int64(limit)
	if end > int64(len(full)) {
		end = int64(len(full))
	}
	return full[start:end], start, nil
}

// ensureFeedEngine 保证打分 ZSET 存在（不存在则重建，同用户单飞防惊群）
func (s *FeedService) ensureFeedEngine(ctx context.Context, key string) error {
	ok, err := s.cache.Exists(ctx, key)
	if err != nil {
		return err
	}
	if ok {
		return nil
	}
	lock := engineLockFor(0)
	lock.Lock()
	defer lock.Unlock()
	ok, _ = s.cache.Exists(ctx, key)
	if ok {
		return nil
	}
	return s.buildFeedEngine(ctx, key)
}

// buildFeedEngine 全量重建打分 ZSET（全部已发布笔记，含用户关注/类型偏好加权）
func (s *FeedService) buildFeedEngine(ctx context.Context, key string) error {
	notes, err := s.repo.ListAllPublished(ctx)
	if err != nil {
		return err
	}
	now := time.Now()
	scores := make(map[int64]int64, len(notes))
	var stats map[int64]*model.NoteStats
	if s.stats != nil {
		ids := make([]int64, len(notes))
		for i, n := range notes {
			ids[i] = n.ID
		}
		stats, _ = s.stats.GetStatsMap(ctx, ids)
	}
	for _, n := range notes {
		scores[n.ID] = foldScore(baseScore(n, now, stats[n.ID]), n.ID)
	}
	return s.cache.ZAddFeed(ctx, key, scores, 0) // 0 = 不过期，靠写路径增量维护
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
	noteIDs := make([]int64, len(notes))
	for i, n := range notes {
		noteIDs[i] = n.ID
	}
	topicsByNote, _ := s.repo.ListTopicsByNoteIDs(ctx, noteIDs)

	items := make([]model.FeedItem, 0, len(notes))
	nextCursor := ""

	for _, note := range notes {
		summary := note.Content
		if note.ContentFormat == ContentFormatRich {
		summary = cursor.StripHTML(summary)
		}
		item := model.FeedItem{
			ID:          note.ID,
			AuthorID:    note.AuthorID,
			Title:       note.Title,
			Content:     cursor.BuildSummary(summary, 120),
			Images:      parseFeedImages(note.Images),
			Type:        note.Type,
			VideoURL:    note.VideoURL,
			Topics:      topicsByNote[note.ID],
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
					u.PasswordHash = ""
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
//增量：新笔记/互动后，单条写入全局基础分 ZSET
func (s *FeedService) UpsertNoteScore(ctx context.Context , noteID int64) error {
	note , err := s.repo.GetScoringFields(ctx , noteID)
	if err != nil {
		return err
	}
	var st *model.NoteStats
	if s.stats != nil {
		if m, _ := s.stats.GetStatsMap(ctx, []int64{noteID}); m != nil {
			st = m[noteID]
		}
	}
	sc := baseScore(note, time.Now(), st)
	return s.cache.ZAddNote(ctx, cache.FeedEngineKey(0), noteID, foldScore(sc, noteID))
}
//删笔记时单条移除
func (s *FeedService) RemoveNoteScore(ctx context.Context, noteID int64) error {
	return s.cache.ZRemNote(ctx, cache.FeedEngineKey(0), noteID)
}
//只重算 24h 内新笔记池
func (s *FeedService) RescoreRecentNotes(ctx context.Context) error {
	since := time.Now().Add(-DecayFreezeHours * time.Hour)
	notes, err := s.repo.ListSince(ctx, since, 5000) // 复用已有 ListSince
	if err != nil {
		return err
	}
	if s.cache == nil || len(notes) == 0 {
		return nil
	}
	now := time.Now()
	ids := make([]int64, len(notes))
	for i, n := range notes {
		ids[i] = n.ID
	}
	var statsMap map[int64]*model.NoteStats
	if s.stats != nil {
		statsMap, _ = s.stats.GetStatsMap(ctx, ids)
	}
	for _, n := range notes {
		sc := baseScore(n, now, statsMap[n.ID])
		_ = s.cache.ZAddNote(ctx, cache.FeedEngineKey(0), n.ID, foldScore(sc, n.ID))
	}
	return nil
}
// 后台周期重算（替代 60s 全量重建）
func (s *FeedService) StartRescorer(ctx context.Context, interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				_ = s.RescoreRecentNotes(ctx)
			}
		}
	}()
}
