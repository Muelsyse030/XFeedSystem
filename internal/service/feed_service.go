package service

import (
	"XFeedSystem/internal/cache"
	"XFeedSystem/internal/model"
	"XFeedSystem/internal/pkg/cursor"
	"XFeedSystem/internal/repo"
	"context"
	"encoding/json"
	"time"
)

const feedCacheTTL = 5 * time.Second

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

    // 2. 首页查缓存（key 要区分用户，因为不同用户看到的内容不同）
    if cursorID == 0 && s.cache != nil {
        var resp FeedListResponse
        cacheKey := cache.FeedForYouKeyV2(currentUserID, limit) // 新 key，区分用户
        if err := s.cache.GetJSON(ctx, cacheKey, &resp); err == nil {
            return &resp, nil
        }
    }

    // 3. 拉候选池
    notes, err := s.repo.ListRecent(ctx, PoolSize)
    if err != nil {
        return nil, err
    }
    notes = s.filterBlockedNotes(ctx, currentUserID, notes)

    // 4. 准备加权数据（仅登录用户）
    var followingSet map[int64]bool
    var typePref map[int8]float64
    if currentUserID > 0 {
        if ids, err := s.getFollowingIDs(ctx, currentUserID); err == nil {
            followingSet = make(map[int64]bool, len(ids))
            for _, id := range ids {
                followingSet[id] = true
            }
        }
        typePref, _ = s.repo.GetUserTypePreference(ctx, currentUserID)
    }

    // 5. 打分排序
    scored := scoreAndSort(notes, time.Now(), followingSet, typePref)

    // 6. 游标分页
    // 如果传了 cursor，找到游标位置，从后面开始取
    startIdx := 0
    if cursorID > 0 {
        for i, sn := range scored {
            if sn.Score < cursorScore || (sn.Score == cursorScore && sn.Note.ID < cursorID) {
                startIdx = i + 1
                break
            }
        }
    }

    // 7. 取 limit 条
    endIdx := startIdx + limit
    if endIdx > len(scored) {
        endIdx = len(scored)
    }
    page := scored[startIdx:endIdx]
    if len(page) == 0 {
        return &FeedListResponse{Items: []model.FeedItem{}, NextCursor: ""}, nil
    }

    // 8. 组装响应（复用现有 buildFeedResponse 逻辑，但输入是 []*model.Note）
    pageNotes := make([]*model.Note, len(page))
    for i, sn := range page {
        pageNotes[i] = sn.Note
    }

    resp, err := s.buildFeedResponse(ctx, pageNotes)
    if err != nil {
        return nil, err
    }

    // 9. 下一页游标 = 本页最后一条的 score + id
    last := page[len(page)-1]
    resp.NextCursor = cursor.EncodeScoreCursor(last.Score, last.Note.ID)

    // 10. 首页写缓存
    if cursorID == 0 && s.cache != nil {
        _ = s.cache.SetJSON(ctx, cache.FeedForYouKeyV2(currentUserID, limit), resp, 5*time.Minute)
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

