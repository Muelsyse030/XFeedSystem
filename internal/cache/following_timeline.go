package cache

import (
	"context"
	"fmt"
	"sort"
	"strconv"

	"github.com/redis/go-redis/v9"
)

// ---------- Following Timeline Redis Key ----------

// FollowingTimelineKey 用户个人时间线：feed:following:{userID}
func FollowingTimelineKey(userID int64) string {
	return fmt.Sprintf("feed:following:%d", userID)
}

// AuthorTimelineKey 作者时间线：feed:author:{authorID}（Celebrity Pull 数据源）
func AuthorTimelineKey(authorID int64) string {
	return fmt.Sprintf("feed:author:%d", authorID)
}

// ---------- 时间线写入 ----------

// TimelineAdd 一次 ZADD 请求；Trim > 0 时在同一条 Pipeline 里裁剪到最近 Trim 条。
// 为什么把 Trim 和 ZADD 放进同一条 Pipeline：先写后裁，中间不存在“超过上限”的中间态，
// 也避免每次 fanout 后单独扫一遍 key 做 ZCARD。
type TimelineAdd struct {
	Key    string
	NoteID int64
	Millis int64 // published_at.UnixMilli()
	Trim   int64 // 0 表示不裁剪
}

// TimelineItem Redis 时间线返回的一条记录（只含 NoteID + 毫秒时间戳，不复制 Note）
type TimelineItem struct {
	NoteID int64
	Millis int64
}

// ZAddTimelineBatch 批量 ZADD + 可选 Trim（Pipeline，一次网络往返）
func (c *RedisCache) ZAddTimelineBatch(ctx context.Context, adds []TimelineAdd) error {
	if len(adds) == 0 {
		return nil
	}
	pipe := c.client.Pipeline()
	for _, a := range adds {
		pipe.ZAdd(ctx, a.Key, redis.Z{
			Score:  float64(a.Millis),
			Member: strconv.FormatInt(a.NoteID, 10),
		})
		if a.Trim > 0 {
			// 保留排名最大的最近 Trim 条（rank 升序=旧到新，裁掉开头的超龄成员）
			pipe.ZRemRangeByRank(ctx, a.Key, 0, -(a.Trim + 1))
		}
	}
	_, err := pipe.Exec(ctx)
	return err
}

// ZRemTimeline 从时间线移除指定 NoteID（幂等）
func (c *RedisCache) ZRemTimeline(ctx context.Context, key string, noteIDs ...int64) error {
	if len(noteIDs) == 0 {
		return nil
	}
	pipe := c.client.Pipeline()
	for _, id := range noteIDs {
		pipe.ZRem(ctx, key, strconv.FormatInt(id, 10))
	}
	_, err := pipe.Exec(ctx)
	return err
}

// ---------- 时间线读取（(published_at, noteID) 键集分页） ----------

// ListTimelinePage 按 (score desc, noteID desc) 取一页。
// 为什么不是只传 score 上界：同一毫秒内 Redis ZSET 的成员顺序是字典序而不是数值序，
// 因此“score 相同再比 noteID”需要单独取同分区间并在内存里按 noteID 数值排序。
func (c *RedisCache) ListTimelinePage(
	ctx context.Context,
	key string,
	cursorMillis int64,
	cursorNoteID int64,
	hasCursor bool,
	limit int,
) ([]TimelineItem, error) {
	if limit <= 0 {
		return nil, nil
	}

	if !hasCursor {
		zs, err := c.client.ZRevRangeByScoreWithScores(ctx, key, &redis.ZRangeBy{
			Max:    "+inf",
			Min:    "-inf",
			Offset: 0,
			Count:  int64(limit + 1), // 多取 1 条，调用方据它推进游标
		}).Result()
		if err != nil {
			return nil, err
		}
		items := timelineItemsFromZ(zs)
		if len(items) > limit {
			items = items[:limit]
		}
		return items, nil
	}

	// Redis ZSET 对同一 score 的成员按“字符串字典序”排序，不是数值 noteID 序，
	// 因此 (score, noteID) 键集分页要分两步：
	// 1) score == cursorMillis 的“同分项”：数值 noteID < cursorNoteID，按 noteID 倒序；
	// 2) score  < cursorMillis 的“更早项”：直接按 score 倒序。
	// 同分项比更早项新，必须先拼同分项、再接更早项，才能保持 (score desc, noteID desc)。
	ms := strconv.FormatInt(cursorMillis, 10)
	eq, err := c.ZRangeByScore(ctx, key, ms, ms, 0, -1)
	if err != nil {
		return nil, err
	}
	var ties []TimelineItem
	for _, z := range eq {
		it := timelineItemFromZ(z)
		if it.NoteID != 0 && it.NoteID < cursorNoteID {
			ties = append(ties, it)
		}
	}
	sort.Slice(ties, func(i, j int) bool {
		return ties[i].NoteID > ties[j].NoteID
	})

	items := make([]TimelineItem, 0, limit+1)
	lowerNeed := int64(limit + 1)
	if len(ties) < int(lowerNeed) {
		lowerNeed -= int64(len(ties))
	} else {
		ties = ties[:limit+1]
		lowerNeed = 0
	}
	items = append(items, ties...)

	if lowerNeed > 0 {
		zs, err := c.client.ZRevRangeByScoreWithScores(ctx, key, &redis.ZRangeBy{
			Max:    fmt.Sprintf("(%d", cursorMillis),
			Min:    "-inf",
			Offset: 0,
			Count:  lowerNeed,
		}).Result()
		if err != nil {
			return nil, err
		}
		items = append(items, timelineItemsFromZ(zs)...)
	}

	if len(items) > limit {
		items = items[:limit]
	}
	return items, nil
}

func timelineItemsFromZ(zs []redis.Z) []TimelineItem {
	items := make([]TimelineItem, 0, len(zs))
	for _, z := range zs {
		if it := timelineItemFromZ(z); it.NoteID != 0 {
			items = append(items, it)
		}
	}
	return items
}

func timelineItemFromZ(z redis.Z) TimelineItem {
	member, ok := z.Member.(string)
	if !ok {
		return TimelineItem{}
	}
	id, err := strconv.ParseInt(member, 10, 64)
	if err != nil {
		return TimelineItem{}
	}
	return TimelineItem{
		NoteID: id,
		Millis: int64(z.Score),
	}
}
