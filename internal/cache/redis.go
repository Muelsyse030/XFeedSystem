package cache

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

var ErrRedisMiss = errors.New("cache miss")

type RedisCache struct {
	client *redis.Client
}

func NewRedisCache(addr, password string, db int) *RedisCache {
	return &RedisCache{
		client: redis.NewClient(&redis.Options{
			Addr:     addr,
			Password: password,
			DB:       db,
		}),
	}
}

func (c *RedisCache) Ping(ctx context.Context) error {
	return c.client.Ping(ctx).Err()
}

func (c *RedisCache) Close() error {
	return c.client.Close()
}

// func (c *RedisCache) GetInt64Slice(ctx context.Context, key string) ([]int64, error) {
// 	val, err := c.client.Get(ctx, key).Result()
// 	if err != nil {
// 		if errors.Is(err, redis.Nil) {
// 			return nil, ErrRedisMiss
// 		}
// 		return nil, err
// 	}
// 	if val == "" {
// 		return []int64{}, nil
// 	}
// 	var ids []int64
// 	if err := json.Unmarshal([]byte(val), &ids); err != nil {
// 		return nil, err
// 	}
// 	return ids, nil
// }

// func (c *RedisCache) SetInt64Slice(ctx context.Context, key string, ids []int64, ttl time.Duration) error {
// 	data, err := json.Marshal(ids)
// 	if err != nil {
// 		return err
// 	}
// 	return c.client.Set(ctx, key, data, ttl).Err()
// }

// func (c *RedisCache) Delete(ctx context.Context, keys ...string) error {
// 	if len(keys) == 0 {
// 		return nil
// 	}
// 	return c.client.Del(ctx, keys...).Err()
// }

func (c *RedisCache) SAdd(ctx context.Context, key string, ids ...int64) error {
	if len(ids) == 0 {
		return nil
	}
	members := make([]interface{}, len(ids))
	for i, id := range ids {
		members[i] = strconv.FormatInt(id, 10)
	}
	return c.client.SAdd(ctx, key, members...).Err()
}

func (c *RedisCache) SRem(ctx context.Context, key string, ids ...int64) error {
	if len(ids) == 0 {
		return nil
	}
	members := make([]interface{}, len(ids))
	for i, id := range ids {
		members[i] = strconv.FormatInt(id, 10)
	}
	return c.client.SRem(ctx, key, members...).Err()
}

func (c *RedisCache) SIsMember(ctx context.Context, key string, id int64) (bool, error) {
	return c.client.SIsMember(ctx, key, strconv.FormatInt(id, 10)).Result()
}

func (c *RedisCache) SMembers(ctx context.Context, key string) ([]int64, error) {
	vals, err := c.client.SMembers(ctx, key).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, ErrRedisMiss
		}
		return nil, err
	}
	if len(vals) == 0 {
		return []int64{}, nil
	}
	ids := make([]int64, 0, len(vals))
	for _, v := range vals {
		id, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			continue
		}
		ids = append(ids, id)
	}
	return ids, nil
}

func (c *RedisCache) Expire(ctx context.Context, key string, ttl time.Duration) error {
	return c.client.Expire(ctx, key, ttl).Err()
}

func FollowingIDsKey(userID int64) string {
	return fmt.Sprintf("follow:following:%d", userID)
}

func (c *RedisCache) Get(ctx context.Context, key string) (string, error) {
	val, err := c.client.Get(ctx, key).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return "", ErrRedisMiss
		}
		return "", err
	}
	return val, nil
}

func (c *RedisCache) Set(ctx context.Context, key string, value string, ttl time.Duration) error {
	return c.client.Set(ctx, key, value, ttl).Err()
}

func (c *RedisCache) Delete(ctx context.Context, keys ...string) error {
	if len(keys) == 0 {
		return nil
	}
	return c.client.Del(ctx, keys...).Err()
}

func (c *RedisCache) GetJSON(ctx context.Context, key string, dest interface{}) error {
	val, err := c.Get(ctx, key)
	if err != nil {
		return err
	}
	return json.Unmarshal([]byte(val), dest)
}

func (c *RedisCache) SetJSON(ctx context.Context, key string, value interface{}, ttl time.Duration) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return c.Set(ctx, key, string(data), ttl)
}

func NoteKey(noteID int64) string {
	return fmt.Sprintf("note:%d", noteID)
}

func FeedForYouKey(limit int) string {
	return fmt.Sprintf("feed:foryou:%d", limit)
}

func UserKey(userID int64) string {
	return fmt.Sprintf("user:%d", userID)
}

func UserNotesKey(authorID int64, limit int) string {
	return fmt.Sprintf("user:%d:notes:%d", authorID, limit)
}

func (c *RedisCache) MGet(ctx context.Context, keys ...string) ([]string, error) {
	if len(keys) == 0 {
		return nil, nil
	}
	vals, err := c.client.MGet(ctx, keys...).Result()
	if err != nil {
		return nil, err
	}
	result := make([]string, len(vals))
	for i, v := range vals {
		if v == nil {
			result[i] = ""
			continue
		}
		result[i] = v.(string)
	}
	return result, nil
}

func (c *RedisCache) MGetJSON(ctx context.Context, keys []string, dest interface{}) (int, error) {
	vals, err := c.MGet(ctx, keys...)
	if err != nil {
		return 0, nil
	}
	rv := reflect.ValueOf(dest).Elem()
	hit := 0
	for _, val := range vals {
		if val == "" {
			continue
		}
		elem := reflect.New(rv.Type().Elem())
		if err := json.Unmarshal([]byte(val), elem.Interface()); err != nil {
			continue
		}
		rv.Set(reflect.Append(rv, elem))
		hit++
	}
	return hit, nil
}

func NotifUnreadKey(userID int64) string {
	return fmt.Sprintf("notif:unread:%d", userID)
}

func (c *RedisCache) Incr(ctx context.Context, key string) (int64, error) {
	return c.client.Incr(ctx, key).Result()
}

func BlockedIDsKey(userID int64) string {
	return fmt.Sprintf("block:blocked:%d", userID)
}

func FeedForYouKeyV2(userID int64 , limit int) string {
	return fmt.Sprintf("feed:foryou:%d:%d", userID, limit)
}

func ScoredPoolKey(userID int64) string {
	return fmt.Sprintf("feed:scoredpool:v1:%d", userID)
}
