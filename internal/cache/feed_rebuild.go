package cache

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

const (
	feedRebuildTimeout = 10 * time.Second
	feedRebuildLockTTL = 15 * time.Second
	feedTempTTL        = time.Minute
	feedCleanupTimeout = time.Second
)

var feedReadyScript = redis.NewScript(`
local state = redis.call("GET", KEYS[2])
if state == "empty" then
	return 1
end
if state == "ready" and redis.call("EXISTS", KEYS[1]) == 1 then
	return 1
end
return 0
`)

var releaseRebuildLockScript = redis.NewScript(`
if redis.call("GET", KEYS[1]) == ARGV[1] then
	return redis.call("DEL", KEYS[1])
end
return 0
`)

var publishFeedScript = redis.NewScript(`
if redis.call("GET", KEYS[1]) ~= ARGV[1] then
	return 0
end

local version = redis.call("GET", KEYS[4]) or "0"
if version ~= ARGV[2] then
	return -1
end

if ARGV[3] == "1" then
	if redis.call("EXISTS", KEYS[3]) == 0 then
		return -2
	end
	redis.call("RENAME", KEYS[3], KEYS[2])
	redis.call("PERSIST", KEYS[2])
	redis.call("SET", KEYS[5], "ready")
else
	redis.call("DEL", KEYS[2])
	redis.call("SET", KEYS[5], "empty")
end

redis.call("DEL", KEYS[1])
return 1
`)

var mutateFeedScript = redis.NewScript(`
if ARGV[1] == "clear" then
	redis.call("DEL", KEYS[1], KEYS[2])
	redis.call("INCR", KEYS[3])
	return 1
end

local state = redis.call("GET", KEYS[2])
local complete =
	state == "empty" or
	(state == "ready" and redis.call("EXISTS", KEYS[1]) == 1)

if ARGV[1] == "add" then
	for i = 2, #ARGV, 2 do
		redis.call("ZADD", KEYS[1], ARGV[i], ARGV[i + 1])
	end
elseif ARGV[1] == "remove" then
	for i = 2, #ARGV do
		redis.call("ZREM", KEYS[1], ARGV[i])
	end
else
	return redis.error_reply("unknown feed operation")
end

redis.call("INCR", KEYS[3])

if complete then
	if redis.call("ZCARD", KEYS[1]) == 0 then
		redis.call("SET", KEYS[2], "empty")
	else
		redis.call("SET", KEYS[2], "ready")
	end
else
	redis.call("DEL", KEYS[2])
end

return 1
`)

type FeedScoreLoader func(context.Context) (map[int64]int64, error)

func (c *RedisCache) globalFeedReady(ctx context.Context, key string) (bool, error) {
	n, err := feedReadyScript.Run(
		ctx, c.client,
		[]string{key, key + ":ready"},
	).Int64()
	return n == 1, err
}

func (c *RedisCache) EnsureGlobalFeed(ctx context.Context, key string, load FeedScoreLoader) error {
	ctx, cancel := context.WithTimeout(ctx, feedRebuildTimeout)
	defer cancel()

	for {
		if err := ctx.Err(); err != nil {
			return err
		}

		ready, err := c.globalFeedReady(ctx, key)
		if err != nil {
			return err
		}
		if ready {
			return nil
		}

		token := uuid.NewString()
		lockKey := key + ":rebuild-lock"

		acquired, err := c.client.SetNX(
			ctx, lockKey, token, feedRebuildLockTTL,
		).Result()
		if err != nil {
			return err
		}

		if acquired {
			done, err := c.rebuildGlobalFeed(ctx, key, token, load)
			if err != nil {
				return err
			}
			if done {
				return nil
			}
		}

		delay := time.Duration(50+rand.IntN(100)) * time.Millisecond
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

func (c *RedisCache) rebuildGlobalFeed(ctx context.Context, key, token string, load FeedScoreLoader) (bool, error) {
	lockKey := key + ":rebuild-lock"
	versionKey := key + ":version"
	tempKey := key + ":building:" + token

	defer func() {
		cleanupCtx, cancel := context.WithTimeout(
			context.WithoutCancel(ctx), feedCleanupTimeout,
		)
		defer cancel()

		_, _ = releaseRebuildLockScript.Run(
			cleanupCtx, c.client, []string{lockKey}, token,
		).Result()
		_ = c.client.Del(cleanupCtx, tempKey).Err()
	}()

	ready, err := c.globalFeedReady(ctx, key)
	if err != nil || ready {
		return ready, err
	}

	version, err := c.client.Get(ctx, versionKey).Result()
	if errors.Is(err, redis.Nil) {
		version = "0"
	} else if err != nil {
		return false, err
	}

	scores, err := load(ctx)
	if err != nil {
		return false, err
	}

	hasData := "0"
	if len(scores) > 0 {
		hasData = "1"
		if err := c.ZAddFeed(ctx, tempKey, scores, feedTempTTL); err != nil {
			return false, err
		}
	}

	result, err := publishFeedScript.Run(
		ctx, c.client,
		[]string{
			lockKey,
			key,
			tempKey,
			versionKey,
			key + ":ready",
		},
		token, version, hasData,
	).Int64()
	if err != nil {
		return false, err
	}

	switch result {
	case 1:
		return true, nil
	case 0, -1:
		return false, nil
	default:
		return false, fmt.Errorf(
			"feed rebuild publication failed: result=%d", result,
		)
	}
}

func (c *RedisCache) PutGlobalFeedScores(
	ctx context.Context,
	key string,
	scores map[int64]int64,
) error {
	if len(scores) == 0 {
		return nil
	}

	args := make([]any, 0, 1+len(scores)*2)
	args = append(args, "add")
	for id, score := range scores {
		args = append(args, score, strconv.FormatInt(id, 10))
	}

	return mutateFeedScript.Run(
		ctx, c.client,
		[]string{key, key + ":ready", key + ":version"},
		args...,
	).Err()
}

func (c *RedisCache) RemoveGlobalFeedNotes(ctx context.Context, key string, noteIDs ...int64) error {
	if len(noteIDs) == 0 {
		return nil
	}

	args := make([]any, 0, 1+len(noteIDs))
	args = append(args, "remove")
	for _, id := range noteIDs {
		args = append(args, strconv.FormatInt(id, 10))
	}

	return mutateFeedScript.Run(
		ctx, c.client,
		[]string{key, key + ":ready", key + ":version"},
		args...,
	).Err()
}

func (c *RedisCache) InvalidateGlobalFeed(ctx context.Context, key string) error {
	return mutateFeedScript.Run(
		ctx, c.client,
		[]string{key, key + ":ready", key + ":version"},
		"clear",
	).Err()
}
