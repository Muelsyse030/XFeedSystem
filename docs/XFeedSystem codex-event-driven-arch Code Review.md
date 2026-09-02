# XFeedSystem `codex/event-driven-arch` Code Review

## 1. Review 结论

本次 Review 针对 `codex/event-driven-arch` 分支进行了完整检查，重点覆盖：

- Event-Driven Architecture
- Transactional Outbox
- Redis Streams
- Consumer Group
- Feed
- Search
- Notification
- Cache
- Worker
- 数据一致性
- 错误处理
- Retry / DLQ
- 部署与可观测性
- 并发与故障恢复

### 总体评价

当前分支已经完成了事件驱动架构的主体设计：

```text
API
 ↓
MySQL
 ↓
Transactional Outbox
 ↓
Outbox Relay
 ↓
Redis Streams
 ↓
┌─────────┬─────────┬─────────┐
│  Feed   │ Search  │ Notify  │
└─────────┴─────────┴─────────┘
```

整体架构方向正确，业务模块之间已经实现了一定程度的解耦。

但是目前还存在一些关键可靠性问题，尤其是：

> **部分业务操作没有做到 Business Data + Outbox Event 的原子提交，同时部分 Consumer 的幂等和错误处理存在消息丢失风险。**

因此当前版本建议定位为：

> **Event-Driven Architecture MVP / 重构版本**

暂不建议未经修复直接作为生产版本。

---

# 2. 问题等级

| 等级 | 含义 |
|---|---|
| P0 | 严重一致性 / 数据丢失问题，合并前必须修复 |
| P1 | 生产环境可靠性问题，强烈建议修复 |
| P2 | 架构完善 / 性能 / 可维护性问题，可后续优化 |

---

# 3. P0 问题

## P0-1：业务数据与 Outbox 没有始终在同一个事务中提交

### 问题

部分 Service 的执行流程类似：

```text
Business DB Write
      ↓
    COMMIT
      ↓
Outbox.Enqueue()
```

而不是：

```text
BEGIN
    Business DB Write
    Outbox.Insert
COMMIT
```

受影响的主要业务包括：

- Note Create
- Note Update
- Note Delete
- Comment Create
- Follow
- Like / Unlike
- Favorite / Unfavorite 等相关操作需要逐项确认

### 风险

如果进程在业务数据提交后、Outbox 写入前崩溃：

```text
MySQL:
    Business Data = 存在

Outbox:
    Event = 不存在
```

那么：

- Feed 不会更新
- Search 不会更新
- Notification 不会触发
- 后续无法通过 Event Replay 自动恢复

最终形成永久数据不一致。

### 建议

引入统一 Unit of Work / Transaction：

```go
err := uow.Transaction(ctx, func(tx *gorm.DB) error {
    noteRepo := noteRepo.WithTx(tx)
    outboxRepo := outboxRepo.WithTx(tx)

    if err := noteRepo.Create(...); err != nil {
        return err
    }

    if err := outboxRepo.Enqueue(...); err != nil {
        return err
    }

    return nil
})
```

核心原则：

```text
Business Write
      +
Outbox Insert
      =
Same MySQL Transaction
```

---

## P0-2：Notification Dedup 存在永久丢消息风险

### 当前模式

```text
SET NX dedup:event_id
        ↓
Create Notification
```

### 问题

如果：

```text
SET NX → 成功
       ↓
Create Notification → 失败
       ↓
Worker Crash
```

事件重新消费时：

```text
SET NX → false
       ↓
Skip
```

最终：

```text
Dedup Key       = 存在
Notification    = 不存在
Event           = 无法再次处理
```

通知会永久丢失。

### 建议

使用数据库唯一约束实现幂等：

```sql
UNIQUE(event_id)
```

然后：

```text
Event
 ↓
BEGIN
 ↓
INSERT Notification
ON DUPLICATE KEY ...
 ↓
COMMIT
 ↓
ACK
```

Redis Dedup 可以作为性能优化，但不应该作为最终正确性保障。

---

## P0-3：Notification 创建失败时可能仍然 ACK Event

### 风险

Consumer 的基本约定应该是：

```text
Handler return nil
    =
业务副作用已经成功提交
```

如果 Notification 创建失败却返回 `nil`：

```text
Create Notification Failed
        ↓
Handler returns nil
        ↓
XACK
        ↓
Event Lost
```

### 建议

统一错误处理：

```go
if err := createNotification(...); err != nil {
    return err
}

return nil
```

最终保证：

```text
success → ACK
failure → retry
```

---

## P0-4：Search Worker 错误地把 DB Error 当成 NotFound

### 问题

Search Worker 获取 Note 时，如果简单使用：

```text
err != nil
    ↓
Delete Search Index
```

那么以下错误：

- MySQL timeout
- MySQL connection failure
- Network error
- DB overload

都会被当成：

```text
Note Not Found
```

### 风险

MySQL 临时故障可能导致错误删除 Meilisearch 中原本正确的数据。

### 建议

严格区分：

```text
RecordNotFound
    ↓
Delete Search Index

Other DB Error
    ↓
Return Error
    ↓
Retry
```

---

## P0-5：Event Matrix 不完整

当前事件体系已经覆盖了一部分核心操作，但需要保证正向 / 反向操作完整。

建议最终 Event Matrix：

| Domain | Event |
|---|---|
| Note | `note.created` |
| Note | `note.updated` |
| Note | `note.deleted` |
| Like | `note.liked` |
| Like | `note.unliked` |
| Favorite | `note.favorited` |
| Favorite | `note.unfavorited` |
| Comment | `comment.created` |
| Comment | `comment.deleted` |
| Follow | `user.followed` |
| Follow | `user.unfollowed` |

尤其需要补齐：

```text
comment.deleted
user.unfollowed
```

以及逐项确认 Like / Favorite 等业务是否全部进入 Outbox。

---

# 4. P1 问题

## P1-1：Outbox Relay 的 `SKIP LOCKED` 没有形成真正的 Claim

当前使用：

```sql
SELECT ...
FOR UPDATE SKIP LOCKED
```

但 Transaction 结束后锁立即释放，没有将 Event 标记成：

```text
processing
```

因此多实例 Relay 仍然可能重复读取同一个 Event。

### 建议

不建议追求 Exactly-Once。

推荐：

```text
At-Least-Once Delivery
        +
Idempotent Consumer
```

允许 Relay 重复发布，但要求 Consumer 能安全处理重复 Event。

---

## P1-2：Outbox 缺少完整 Retry / Backoff / DLQ

当前已有 attempts 机制，但缺少完整的失败处理策略。

建议：

```text
pending
  ↓
retry
  ↓
exponential backoff
  ↓
max attempts
  ↓
failed / DLQ
```

需要至少具备：

- 最大重试次数
- Retry Backoff
- Failed 状态
- Dead Letter Queue
- 告警

---

## P1-3：Consumer 缺少 Poison Message 处理

例如 Event JSON 损坏：

```text
JSON Decode Error
       ↓
Log
       ↓
ACK
```

这样 Event 会直接永久丢失。

建议：

```text
Invalid Event
     ↓
DLQ
     ↓
ACK Original Event
```

避免坏消息阻塞 Consumer，同时保留问题事件用于排查。

---

## P1-4：Pending Message 需要最大重试次数

当前已经使用 `XAutoClaim` 进行 Pending Message 恢复，这是正确方向。

但需要增加：

```text
delivery_count
```

最终：

```text
Pending
  ↓
Retry
  ↓
Retry
  ↓
Retry
  ↓
超过阈值
  ↓
DLQ
```

否则 Poison Message 可能无限重试。

---

## P1-5：Stats Flush 存在计数丢失 Race

当前模式：

```text
GET Redis Counter
      ↓
写 MySQL
      ↓
DEL Redis Counter
```

可能发生：

```text
Flush:
GET = 100

Request:
INCR +1

Flush:
DEL
```

最终：

```text
MySQL = 100
```

但实际应该：

```text
101
```

### 建议

使用原子 Counter Flush，例如：

- Redis Lua
- Rename Bucket
- Atomic Swap
- 独立 Flush Bucket

推荐 Bucket Rename：

```text
stats:imp:123
       ↓
rename
       ↓
stats:flush:<uuid>:imp:123
```

新的请求继续写：

```text
stats:imp:123
```

这样 Flush 期间不会吞掉新产生的计数。

---

## P1-6：Worker 尚未成为正式部署组件

目前 Worker 已经是整个 Event-Driven 架构的核心组件：

```text
Outbox
 ↓
Redis Stream
 ↓
Worker
```

但部署流程没有完全把 Worker 纳入标准 Deployment。

### 风险

可能出现：

```text
API 正常
MySQL 正常
Redis 正常
Worker 未运行
```

最终：

```text
Outbox 不断增长
Feed 不更新
Search 不更新
Notification 不触发
```

### 建议

正式部署：

```text
xfeed-api
xfeed-worker
```

并分别具备：

- Deployment
- Health Check
- Logging
- Metrics
- Restart Policy

---

## P1-7：API / Worker 启动时执行 AutoMigrate

当前 API 和 Worker 都可能执行 Schema Migration。

多实例环境：

```text
API 1 ─┐
API 2 ─┤
API 3 ─┤
Worker ─┤ → AutoMigrate
Worker ─┘
```

可能产生 migration 竞争。

### 建议

生产环境：

```text
Migration Job
     ↓
DB Migration
     ↓
API / Worker
```

业务进程不要负责生产环境 Schema Migration。

---

## P1-8：Migrations 与 AutoMigrate 职责重复

项目同时存在：

```text
migrations/*.sql
```

以及：

```text
AutoMigrate(...)
```

建议确定唯一 Schema Source of Truth：

```text
migrations/
    ↓
Production Schema
```

`AutoMigrate` 可以仅用于开发环境，或者彻底移除。

---

## P1-9：Worker Shutdown 不够优雅

当前主要依赖：

```text
cancel()
sleep(2s)
```

存在 Worker 在处理 Event 时被强制退出的风险。

建议：

```text
Signal
  ↓
Stop accepting new messages
  ↓
Wait current handlers
  ↓
ACK successful messages
  ↓
Exit
```

实现上建议使用：

- `errgroup`
- `sync.WaitGroup`
- Context Cancellation

---

## P1-10：缺少 Event Pipeline Metrics

Event-Driven 系统必须能够观察：

```text
API
 ↓
Outbox
 ↓
Relay
 ↓
Stream
 ↓
Consumer
```

建议增加：

```text
outbox_pending_count
outbox_oldest_age

events_published_total
events_publish_failed_total

events_processed_total
events_failed_total
events_retried_total
events_dlq_total

consumer_pending_count

event_processing_latency
```

并按照：

```text
event_type
consumer_group
```

进行统计。

---

## P1-11：Worker 缺少 Health / Heartbeat

API Health 只能说明：

```text
MySQL OK
Redis OK
Meilisearch OK
```

不能证明：

```text
Feed Worker OK
Search Worker OK
Notify Worker OK
```

建议增加：

```text
worker_last_success
worker_last_error
consumer_pending
outbox_pending
```

或者独立 Worker Health Endpoint。

---

## P1-12：Context 传递不完整

部分业务代码使用：

```go
context.Background()
```

替代当前 Request Context。

建议：

```text
HTTP Context
    ↓
Handler
    ↓
Service
    ↓
Repository
    ↓
Outbox / Redis / DB
```

全链路传递 Context。

---

## P1-13：Feed Block 查询存在 N+1 风险

如果对每个 Author 单独执行 Block 查询：

```text
100 candidates
     ↓
100 DB queries
```

会产生明显的 N+1。

### 建议

一次获取：

```text
Blocked Author IDs
```

然后：

```go
blockedMap[authorID]
```

在内存中完成过滤。

---

## P1-14：Consumer Name 需要更可靠的唯一性

目前主要依赖：

```text
hostname + pid
```

容器环境中可能存在重复。

建议：

```text
hostname + pid + UUID
```

确保每个 Consumer Instance 都有唯一 ID。

---

# 5. P2 问题

## P2-1：Event Payload 过于通用

当前 Payload 包含大量不同 Event 使用的字段：

```text
NoteID
AuthorID
ActorID
CommentID
ParentID
ReplyToUserID
...
```

随着 Event 增加会越来越臃肿。

后续建议：

```text
EventEnvelope
    +
Typed Payload
```

例如：

```text
NoteCreatedPayload
NoteUpdatedPayload
CommentCreatedPayload
UserFollowedPayload
```

---

## P2-2：Event Version 机制需要完善

目前已有 `Version` 字段，但缺少完整 Schema Evolution 方案。

建议 Event Envelope 标准化：

```json
{
  "event_id": "...",
  "event_type": "note.created",
  "event_version": 1,
  "occurred_at": "...",
  "aggregate_type": "note",
  "aggregate_id": "...",
  "actor_id": "...",
  "payload": {}
}
```

未来需要考虑：

- Backward Compatibility
- Version Migration
- Old Consumer Compatibility

---

## P2-3：Feed Candidate Limit 需要与架构文档保持一致

当前 Feed 实际存在 Candidate Limit。

需要明确：

```text
Global ZSET
    ↓
Candidate Limit
    ↓
Personalization
    ↓
Ranking
    ↓
MMR
```

否则文档容易让人误解为每次请求都处理完整 ZSET。

---

## P2-4：Feed 当前方案存在规模扩展瓶颈

当前模型：

```text
Global ZSET
    ↓
大量 Candidates
    ↓
Personalization
    ↓
Sort
    ↓
MMR
```

在数百 / 数千条内容时没有问题。

如果未来增长到：

```text
100K+
1M+
```

需要演进成：

```text
Candidate Generation
       ↓
Top-N
       ↓
Personalized Ranking
       ↓
MMR
```

避免每次请求处理全部候选。

---

## P2-5：Feed Score 需要增加边界测试

Score Fold 使用：

```text
score × 10000
        +
noteID
```

需要增加：

- 最大 Score
- 最小 Score
- Overflow
- Negative Score
- Fold / Unfold Round Trip

等测试。

---

# 6. 推荐目标架构

最终建议稳定为：

```text
                         ┌─────────────┐
                         │     API     │
                         └──────┬──────┘
                                │
                                ▼
                         ┌─────────────┐
                         │   MySQL     │
                         │             │
                         │ Business DB │
                         │      +      │
                         │   Outbox    │
                         └──────┬──────┘
                                │
                           Same TX
                                │
                                ▼
                         ┌─────────────┐
                         │ Outbox Relay│
                         └──────┬──────┘
                                │
                                ▼
                      ┌───────────────────┐
                      │  Redis Streams    │
                      └─────┬───┬───┬─────┘
                            │   │   │
                   ┌────────┘   │   └────────┐
                   ▼            ▼            ▼
                 Feed         Search       Notify
                   │            │            │
                   ▼            ▼            ▼
                 Redis        Meili         MySQL
```

可靠性模型：

```text
Business DB
     +
Outbox
     ↓
At-Least-Once Delivery
     ↓
Redis Streams
     ↓
Idempotent Consumer
     │
     ├── Success
     │      ↓
     │     ACK
     │
     ├── Transient Error
     │      ↓
     │     Retry
     │
     └── Permanent Error
            ↓
           DLQ
```

---

# 7. 推荐修复顺序

## Phase 1：Transactional Outbox

优先修复：

```text
Note Create
Note Update
Note Delete

Comment Create
Comment Delete

Follow
Unfollow

Like
Unlike

Favorite
Unfavorite
```

全部保证：

```text
Business Write + Outbox Insert
            ↓
       Same Transaction
```

---

## Phase 2：Consumer Contract

统一约定：

```text
Handler return nil
=
Business Side Effect 已成功提交
```

处理模型：

```text
Success
  → ACK

Transient Error
  → Retry

Permanent Error
  → DLQ
```

---

## Phase 3：Consumer Idempotency

### Feed

使用 Redis 原子操作：

```text
ZADD
ZREM
```

保证重复 Event 不会产生错误结果。

### Search

保证：

```text
Index(id)
Delete(id)
```

重复执行不会改变最终状态。

### Notify

使用：

```text
UNIQUE(event_id)
```

实现数据库级幂等。

---

## Phase 4：可靠性基础设施

补齐：

```text
Retry
Backoff
DLQ
Metrics
Health
Heartbeat
Graceful Shutdown
```

---

## Phase 5：故障测试

必须覆盖：

### Outbox

```text
DB Commit 后 API Crash
Outbox Insert 后 API Crash
Relay Crash
Redis Down
```

### Consumer

```text
Worker Crash Before Processing
Worker Crash After Processing
Worker Crash Before ACK
MySQL Down
Meilisearch Down
Malformed Event
Poison Message
```

### 并发

```text
Multiple API Instances
Multiple Relay Instances
Multiple Consumer Instances
Concurrent Stats Flush
Concurrent Feed Rescore
```

---

# 8. P0 / P1 Checklist

## P0

- [ ] 所有 Business Write + Outbox Insert 使用同一个 DB Transaction
- [ ] Note Create / Update / Delete 全部进入 Outbox
- [ ] Comment Create / Delete 事件完整
- [ ] Follow / Unfollow 事件完整
- [ ] Like / Unlike 事件完整
- [ ] Favorite / Unfavorite 事件完整
- [ ] Notification 使用 DB 唯一约束实现幂等
- [ ] Notification Create Error 正确返回
- [ ] Search 正确区分 NotFound 与 DB Error

## P1

- [ ] Outbox Retry
- [ ] Outbox Backoff
- [ ] Outbox DLQ
- [ ] Consumer Retry
- [ ] Consumer DLQ
- [ ] Poison Message Handling
- [ ] Stats Flush 原子化
- [ ] Worker 正式部署
- [ ] Worker Graceful Shutdown
- [ ] Worker Health / Heartbeat
- [ ] Event Pipeline Metrics
- [ ] Context 全链路传递
- [ ] Feed Block 查询消除 N+1
- [ ] Consumer ID 唯一化
- [ ] Production Migration 独立化

## P2

- [ ] Typed Event Payload
- [ ] Event Version Evolution
- [ ] Feed Candidate Limit 文档统一
- [ ] Feed 大规模 Candidate Generation
- [ ] Score Overflow / Boundary Tests
- [ ] 完善 Integration Tests
- [ ] 增加 Failure Injection Tests

---

# 9. 最终评价

当前分支的核心架构**不需要推翻重做**。

值得保留的设计：

```text
Transactional Outbox
        +
Redis Streams
        +
Independent Workers
        +
Materialized Views
```

当前最大的缺口是可靠性闭环：

```text
Business Transaction
        ↓
Transactional Outbox
        ↓
Relay
        ↓
At-Least-Once Stream
        ↓
Idempotent Consumer
        ↓
Retry / DLQ
        ↓
Observability
```

目前这条链已经基本搭起来，但仍存在几个关键断点。

### 优先级结论

```text
P0
↓
解决数据一致性和消息丢失问题

P1
↓
完善 Retry / DLQ / Worker / Metrics

P2
↓
完善 Event Schema 和 Feed 扩展能力
```

**完成 P0 后，架构才具备可靠的 Event-Driven 基础；完成核心 P1 后，才具备生产化条件。**