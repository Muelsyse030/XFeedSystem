# Xenflow 第二轮压测报告（优化代码部署后）

**日期**：2026-08-30
**测试服务器**：81.68.82.243（8 vCPU / 15G，nginx → xfeed-api@1/2（8001/8002）+ xfeed-worker）
**部署内容**：develop 分支优化代码（Sprint 1：FeedCandidateLimit=200、DiversityInputLimit=50、diverseRank O(N log N) 重写），pprof 已开启
**数据规模**：用户 1083、笔记 3194（较上轮 612 增加约 5 倍）、主题 11、测试账号 90000-90049
**压测方式**：负载生成器放在服务器本地回环（127.0.0.1/api，排除公网干扰），80% 读 / 20% 写，与上轮报告口径一致

---

## 一、混合场景结果（修正后完整 13 接口）

| 并发 | QPS | p50 | p95 | p99 | 最大 | 错误率 |
|---|---:|---:|---:|---:|---:|---:|
| 20 | 3,767 | 2ms | 18ms | 23ms | 95ms | 2.80%* |
| 50 | 4,476 | 4ms | 39ms | 51ms | 141ms | 2.79%* |
| 100 | 4,892 | 9ms | 72ms | 100ms | 350ms | 2.78%* |
| 150 | 5,019 | 12ms | 107ms | 183ms | 907ms | 2.89%* |
| 200 | 5,380 | 13ms | 132ms | 346ms | 1,625ms | 8.23% |
| 300 | 5,813 | 16ms | 207ms | 418ms | 1,825ms | 10.53% |

\* 其中约 2.8% 为固定"假错误"：压测随机笔记 ID 大多不存在，而 `/notes/:id` 对不存在的笔记返回 500（应为 404），属于接口缺陷，不是负载问题。扣除后，150 并发以内真实错误率接近 0。

### 与上轮（旧代码、612 笔记）对比

| 指标 | 旧版 20c | 新版 20c | 旧版 50c | 新版 50c |
|---|---:|---:|---:|---:|
| QPS | 1,240 | 3,767（3.0x） | 671 | 4,476（6.7x） |
| p50 | 4ms | 2ms | 24ms | 4ms |
| p99 | 117ms | 23ms | 510ms | 51ms |

旧版在 50 并发触发熔断停机；新版 300 并发仍可运行（错误率 10.5%、p99 418ms，未触发熔断）。

## 二、单接口结果（50 并发，60s）

| 接口 | QPS | p50 | p95 | p99 | 说明 |
|---|---:|---:|---:|---:|---|
| GET /feed（匿名） | 6,720 | 2ms | 43ms | 55ms | 首页 |
| GET /feed（登录） | 9,119 | 1ms | 4ms | **297ms** | p99 长尾来自用户画像查询 |
| GET /notes/:id | 43,500 | 1ms | 2ms | 3ms | 命中 30s 字节缓存 |
| GET /topics/hot | 40,816 | 0ms | 2ms | 4ms | 5min JSON 缓存 |
| GET /topics/:id/feed | 53,843 | 0ms | 2ms | 3ms | |
| GET /search | 2,667 | 18ms | 27ms | 32ms | **最慢读接口**（Meilisearch） |
| GET /users/:id | 56,952 | 0ms | 1ms | 2ms | |
| GET /users/:id/notes | 44,043 | 0ms | 2ms | 4ms | |
| POST /notes/:id/like | 3,017 | 13ms | 35ms | 50ms | 含去重：首轮后多为空操作 |
| POST /notes/:id/favorite | 3,103 | 13ms | 34ms | 47ms | 同上 |
| POST /notes/:id/comments | **162** | **309ms** | 339ms | 350ms | **真写瓶颈**：热行锁 |
| POST /users/:id/follow | 4,909 | 8ms | 23ms | 29ms | |
| POST /notes（发帖） | 2,235 | 13ms | 18ms | 25ms | 30 并发实测 |

## 三、pprof 热点（优化前后对比）

| 函数 | 旧版（报告） | 新版 50c 混合 | 新版 150c 混合 |
|---|---:|---:|---:|
| diverseRank（cum） | 82.86% | 不在 Top10 | 不在 Top12 |
| syscall.Syscall6 | - | 15.44% | 17.81% |
| GC（mallocgc/scanobject 等合计） | - | ~10% | ~16% |

结论：**CPU 热点已消除，系统进入 I/O 瓶颈阶段**（系统调用占首位，即网络/Redis/MySQL 往返），其次是大对象分配（GC）。`diverseRank` 从 82.86% 掉出前十，Sprint 1 目标达成。

## 四、本轮发现的瓶颈与问题

1. **登录态 /feed 的 p99 长尾（297ms）**：登录用户每次请求仍在热路径查询类型偏好（3 条 UNION SQL）、关注话题、隐藏笔记、逐个作者拉黑判断。这正是报告 Sprint 2（用户画像 Redis 化 + Block 批量判断）要解决的问题。匿名 /feed 的 p99 只有 55ms。
2. **评论接口 162 QPS / p50 309ms**：所有评论都走"INSERT + UPDATE notes SET comment_count+1 + outbox"同一事务，并发打在同一个热笔记上时行锁排队。点赞/收藏看着快（3000 QPS）是因为 `OnConflict DoNothing` 去重，首轮之后全是空操作；评论是唯一每请求真实写库的接口。
3. **200 并发以上错误率上升**：MySQL 峰值连接 152/200、`Innodb_row_lock_current_waits=23`，写路径行锁 + 连接池接近上限。
4. **/notes/:id 对不存在 ID 返回 500**：`note_handler.go` 把 `ErrRecordNotFound` 当 500 处理，压测里 2.8% 的"错误"全是它，应返回 404。
5. **搜索是最慢读接口（2667 QPS）**：依赖 Meilisearch，p50 18ms，属于正常但值得关注。
6. **Redis 表现良好**：keyspace 命中率约 97.7%（29.7M hits / 0.7M misses）。

## 五、建议的下一步

### 优先（对应报告 Sprint 2/3）

1. **用户画像 Redis 化 + Block 批量判断**：登录 /feed 的 MySQL 往返从 5-6 条降到 1-2 条，消除 297ms 长尾（缓存键和方法代码已就绪）。
2. **修复 /notes/:id 404**：一行错误分支，消除压测假错误，让错误率指标恢复真实。
3. **Note 缓存 + 每用户 Feed 排名缓存**：降低 GC 分配与系统调用（当前 GC 已占 16%）。

### 架构级

4. **评论计数与插入解耦**：把 `UPDATE notes SET comment_count+1` 移出评论事务（事件/Redis 异步累加，worker 落库），消除热行锁。这是当前唯一的真写瓶颈。
5. **搜索结果缓存**：高频关键词加 10-30s 缓存，Search QPS 可数倍提升。
6. **连接池收敛与监控**：MySQL 峰值 152/200 偏高，压测时接入 slow query 与 lock wait 监控。

## 六、验收对照

| 指标 | 上轮目标 | 实际 |
|---|---:|---:|
| 50 并发 QPS | ≥ 1,200 | 4,476 |
| 50 并发 p99 | < 300ms | 51ms |
| diverseRank CPU | < 30% | 不在 Top10 |
| Candidate | ≤ 200-300 | 200 |
| Diversity 输入 | ≤ 50-100 | 50 |
