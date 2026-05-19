package cache

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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

func (c *RedisCache) GetInt64Slice(ctx context.Context, key string) ([]int64, error) {
	val, err := c.client.Get(ctx, key).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, ErrRedisMiss
		}
		return nil, err
	}
	if val == "" {
		return []int64{}, nil
	}
	var ids []int64
	if err := json.Unmarshal([]byte(val), &ids); err != nil {
		return nil, err
	}
	return ids, nil
}

func (c *RedisCache) SetInt64Slice(ctx context.Context, key string, ids []int64, ttl time.Duration) error {
	data, err := json.Marshal(ids)
	if err != nil {
		return err
	}
	return c.client.Set(ctx, key, data, ttl).Err()
}

func (c *RedisCache) Delete(ctx context.Context, keys ...string) error {
	if len(keys) == 0 {
		return nil
	}
	return c.client.Del(ctx, keys...).Err()
}

func FollowingIDsKey(userID int64) string {
	return fmt.Sprintf("follow:following:%d", userID)
}
