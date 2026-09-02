# XFeedSystem 高并发计数优化方案总结

## 1. 当前系统现状

XFeedSystem 当前已经具备比较完整的 Feed 架构：

- Following：关注流，按时间倒序获取内容，属于 Timeline Feed。
- For You：基于 Redis ZSET 的候选集 + 用户画像 + 个性化打分 + Diversity 的推荐 Feed。
- 事件驱动：MySQL 事务性 Outbox + Redis Streams + Feed/Search/Notify Worker。
- Redis：用于 Feed ZSET、用户画像、缓存、曝光/阅读统计等。
- MySQL：保存用户、笔记、点赞、收藏、评论等核心业务数据。

当前 Feed CPU 算法瓶颈已经基本解决，下一阶段重点应该放在 **I/O、MySQL 写路径和热点计数**。

## 2. 第三轮压测暴露的核心问题

### 2.1 混合压测结果

| 并发 | QPS | p50 | p95 | p99 | 错误率 |
|---:|---:|---:|---:|---:|---:|
| 20 | 3746 | 1ms | 18ms | 47ms | 0% |
| 50 | 4233 | 4ms | 39ms | 100ms | 0% |
| 100 | 4490 | 8ms | 81ms | 182ms | 0% |
| 150 | 4640 | 8ms | 133ms | 390ms | 0.66% |
| 200 | 5475 | 11ms | 136ms | 407ms | 8.11% |
| 300 | 5670 | 11ms | 231ms | 581ms | 9.99% |

150 并发以内基本稳定，200+ 并发开始明显出现错误。

### 2.2 pprof 结论

第三轮 pprof 表明：

- `diverseRank` 已经退出主要 CPU 热点。
- `syscall.Syscall6` 在高并发下达到 20% 以上。
- GC 约占 10%～13%。
- 当前系统已经从 CPU 密集型问题转向 **I/O 瓶颈**。

因此，继续花大量时间优化 Diversity 算法的收益已经很低。

## 3. 真正的瓶颈：热点计数行锁

当前点赞、收藏、评论流程中，核心业务记录和 `notes` 计数器更新位于同一 MySQL 事务中。

以点赞为例：

```text
BEGIN
  INSERT note_likes
  UPDATE notes SET like_count = like_count + 1
  INSERT outbox_events
COMMIT
```

收藏、评论也存在类似模式。

当大量用户同时操作同一篇热门笔记时：

```text
User A ─┐
User B ─┤
User C ─┤
...     ├──> UPDATE notes WHERE id = 热门Note
User N ─┘
```

大量请求竞争同一行，导致：

1. MySQL 行锁等待；
2. 事务持有连接时间变长；
3. 数据库连接池出现等待；
4. p95/p99 延迟增加；
5. 200+ 并发错误率快速上升。

# 4. 第一优先级优化：计数器 Redis 异步化

## 4.1 核心目标

把：

```text
同步更新 notes.like_count
```

改成：

```text
Redis Counter + Worker 异步聚合 + MySQL 批量落库
```

新的请求路径：

```text
用户点赞
   │
   ├── MySQL：INSERT note_likes
   └── MySQL：INSERT outbox_events
             │
           COMMIT
             │
         HTTP 返回
             │
             ▼
       Redis Streams
             │
             ▼
       Counter Worker
             │
             ▼
       Redis INCRBY/DECRBY
             │
             ▼
        定期批量 Flush
             │
             ▼
           MySQL
```

这样用户请求不再直接竞争 `notes` 热点计数行。

# 5. Redis Counter 设计

建议增加以下 Key：

```text
counter:like:{noteID}
counter:favorite:{noteID}
counter:comment:{noteID}
```

推荐把这些 Key 定义为 **增量计数器**，而不是绝对值。

例如：

```text
counter:like:100 = 153
```

表示当前尚未刷入 MySQL 的点赞增量为 153。

MySQL 仍然保存最终持久化统计值：

```text
notes.like_count
notes.favorite_count
notes.comment_count
```

# 6. Counter Worker

现有系统已经有 Outbox + Redis Streams + Worker，因此不需要新增一种 MQ。

建议新增独立消费组：

```text
xfeed:counter
```

事件处理关系：

| 事件 | Counter |
|---|---:|
| NoteLiked | like +1 |
| NoteUnliked | like -1 |
| NoteFavorited | favorite +1 |
| NoteUnfavorited | favorite -1 |
| CommentCreated | comment +1（一级评论） |
| CommentDeleted | comment -1（一级评论） |

需要保持当前业务语义：

> `parent_id = 0` 的一级评论才影响 `comment_count`，回复评论不直接增加笔记评论总数。

# 7. 不要使用 GET + DEL 做 Flush

错误方案：

```text
GET counter
UPDATE MySQL
DEL counter
```

存在并发丢计数问题。

例如：

```text
Worker GET = 100

用户又 INCR 1
Redis = 101

Worker 写 MySQL +100
Worker DEL

最终 +1 丢失
```

## 正确方案

使用 Redis Lua 或其他原子操作：

```text
读取当前增量
+
把当前增量原子置零
+
返回旧增量
```

逻辑：

```lua
local v = redis.call('GET', KEYS[1])
if not v then
    return 0
end

redis.call('SET', KEYS[1], '0')
return v
```

这样新产生的计数会进入下一批，不会被误删。

# 8. 批量 Flush MySQL

推荐：

```text
每 1～5 秒 Flush 一次
每批处理约 100～500 个 counter
```

流程：

```text
SCAN counter keys
       ↓
Lua 原子 drain
       ↓
内存聚合
       ↓
批量 UPDATE MySQL
```

不要：

```text
500 个 counter
→ 500 次 Redis GET
→ 500 次 MySQL UPDATE
→ 500 次 DEL
```

应该尽量减少网络往返和数据库调用。

# 9. 为什么这个优化收益最大

现在 1000 次点赞可能产生：

```text
1000 × INSERT note_likes
+
1000 × UPDATE notes.like_count
+
1000 × Outbox
```

其中 `UPDATE notes` 会竞争同一个热点行。

改造后：

```text
1000 × INSERT note_likes
+
1000 × Outbox
+
1000 × Redis INCRBY
```

最终只需要批量把增量刷入 MySQL。

核心变化：

```text
1000 次热点 UPDATE
        ↓
       1批
```

因此可以同时降低：

- 行锁竞争
- MySQL 连接占用时间
- 事务时长
- 数据库往返次数
- 高并发错误率

# 10. 第二优先级：减少互动请求的 SQL

当前点赞流程在进入真正点赞事务前还会读取 Note，以获得作者信息并进行拉黑校验。

理想目标是减少：

```text
SELECT note
→ INSERT like
→ UPDATE counter
→ INSERT outbox
```

改造成：

```text
必要的数据校验
→ INSERT like
→ INSERT outbox
```

在业务允许的情况下，可以利用已有的 Note Redis Cache 减少不必要的 MySQL 查询。

当前系统已经对 Note 做了 Redis 缓存，因此这一方向可以继续利用现有基础设施。

# 11. 第三优先级：连接池优化

当前数据库配置：

```go
SetMaxOpenConns(100)
SetMaxIdleConns(20)
SetConnMaxLifetime(5 * time.Minute)
```

不要简单地把 MaxOpenConns 从 100 调到 500。

应该先观察：

```text
OpenConnections
InUse
Idle
WaitCount
WaitDuration
```

重点关注：

```text
WaitCount
WaitDuration
```

如果 150 → 200 并发时二者明显暴涨，说明连接池确实成为瓶颈。

正确顺序：

```text
先消除热点行锁
        ↓
再重新测试连接池
        ↓
根据 WaitCount/WaitDuration 调参
```

# 12. 第四优先级：搜索缓存

当前 `/search` 是最慢的读接口：

```text
QPS ≈ 2009
p95 ≈ 35ms
p99 ≈ 42ms
```

高频关键词可以增加：

```text
search:{queryHash}
```

例如：

```text
TTL = 10～30 秒
```

流程：

```text
GET /search?q=redis
       ↓
Redis
       │
   hit  │  miss
       │    ↓
       │ Meilisearch
       ↓
     返回
```

这是低风险、容易实现的优化，但优先级低于热点计数器。

# 13. 第五优先级：Pipeline

当前系统已经提供 Redis Pipeline 能力，例如批量 ZSET、批量 INCR 等。

后续应继续把：

```text
多次独立 Redis RTT
```

改成：

```text
一次 Pipeline
```

适合的场景包括：

- Counter 批量更新；
- Feed 批量获取；
- 缓存批量失效；
- 批量用户/笔记读取。

当前 pprof 已表明系统主要进入 I/O 瓶颈，因此减少网络往返有实际价值。

# 14. 第六优先级：GC 优化

当前 GC：

```text
50 并发：约 6%
150 并发：约 13%
登录 /feed：约 10%
```

属于次要瓶颈。

建议先用：

```text
pprof alloc
pprof heap
```

定位真正高分配对象，再针对性优化：

- slice 复用；
- 减少临时 map；
- 减少 JSON 编解码；
- 必要时使用 `sync.Pool`。

不要在没有 profiling 证据时大量使用 `sync.Pool`。

# 15. 第七优先级：powf 微优化

Diversity 中存在：

```text
powf
```

登录 `/feed` 场景约占 5.81% CPU。

可以将：

```text
惩罚基数^次数
```

预计算为表：

```go
var powTable [32]float64
```

然后直接数组查表。

这是微优化，预计收益只有几个百分点，因此不要把它作为下一阶段核心工作。

# 16. 第四轮压测方案

## A. 随机笔记测试

测试：

```text
100 / 150 / 200 / 300 / 500 并发
```

观察：

- QPS
- p50
- p95
- p99
- 错误率
- MySQL connections
- Redis ops

## B. 单热点笔记测试

所有请求都操作同一个 Note：

```text
POST /notes/100/like
```

这是最重要的验证项。

目标是确认：

```text
旧方案：
热点行锁 → 排队 → 错误率上升

新方案：
Redis Counter → 异步聚合 → MySQL 批量落库
```

# 17. 第四轮优化目标

第三轮：

```text
150c → 0.66%
200c → 8.11%
300c → 9.99%
```

下一阶段优先目标不应该是盲目追求峰值 QPS，而是：

```text
150c → 0% 错误
200c → <1%
300c → <1%
500c → 保持可接受延迟
```

当错误率和长尾延迟稳定后，再继续冲 QPS。

# 18. 推荐的 V5 架构

```text
                    XFeedSystem
                         │
          ┌──────────────┴──────────────┐
          │                             │
      Timeline Feed                For You Feed
          │                             │
      时间倒序                     Ranking + MMR
          │                             │
          └──────────────┬──────────────┘
                         │
                       Redis
              ┌──────────┼──────────┐
              │          │          │
            Cache      ZSET       Counter
              │          │          │
              └──────────┼──────────┘
                         │
                 Redis Streams
                         │
       ┌─────────┬───────┼─────────┐
       ↓         ↓       ↓         ↓
     Feed     Search   Notify    Counter
    Worker     Worker   Worker    Worker
                                  │
                                  ↓
                              Batch Flush
                                  │
                                  ↓
                                MySQL
```

# 19. 最终改造优先级

| 优先级 | 项目 | 建议 |
|---|---|---|
| ★★★★★ | 点赞/收藏/评论计数 Redis 化 | 立即做 |
| ★★★★★ | 去除同步 `UPDATE notes counter` | 立即做 |
| ★★★★★ | Counter Worker | 立即做 |
| ★★★★★ | Lua 原子 drain | 立即做 |
| ★★★★★ | MySQL 批量 Flush | 立即做 |
| ★★★★☆ | SQL / Redis RTT 减少 | 第二阶段 |
| ★★★★☆ | 连接池调优 | 第二阶段 |
| ★★★☆☆ | Search Redis Cache | 第二阶段 |
| ★★★☆☆ | Pipeline | 第二阶段 |
| ★★☆☆☆ | GC 优化 | 第三阶段 |
| ★☆☆☆☆ | powf 查表 | 最后做 |

# 20. 一句话结论

当前系统已经完成了 **Feed 排序 CPU 优化**，下一阶段的核心不是继续优化 `diverseRank`，而是：

> **把点赞、收藏、评论的同步数据库计数更新改造成 Redis Counter + Outbox/Redis Streams + Counter Worker + 原子 Drain + 批量 MySQL Flush。**

这会直接针对第三轮压测中已经被定位出来的**热点行锁和连接池压力**，也是最有希望把 200～300 并发错误率显著压下来的改造方向。

## 参考依据

- 第三轮压测报告：Xenflow 第三轮压测报告，2026-08-30。
- XFeedSystem `internal/service/feed_service.go`：Following / For You Feed 实现、推荐候选、用户个性化排序、Diversity 与排名缓存。
- XFeedSystem `internal/service/stats_service.go`：现有 Redis 统计计数与定时落库机制。
- XFeedSystem `internal/cache/redis.go`：Redis Counter、Pipeline、ZSET、MGet 等基础能力。
- XFeedSystem `internal/repo/note_repo.go`：点赞、收藏、评论事务及 `notes` 计数器同步更新实现。
- XFeedSystem `configs/db.go`：MySQL 连接池配置。
