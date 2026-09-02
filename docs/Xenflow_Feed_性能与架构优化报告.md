# Xenflow Feed 系统性能与架构优化报告

**报告日期：** 2026-08-30  
**项目：** Xenflow 内容社区  
**代码分支：** `develop`  
**测试技术栈：** Go + Gin + MySQL + Redis + Meilisearch  
**依据：** Xenflow 压测报告、`develop` 分支 Feed 相关源码分析

---

## 1. 执行摘要

本次分析针对 Xenflow 当前 Feed 系统的压测表现与核心代码架构进行分析。

压测结果显示，新服务器（8 vCPU / 15 GB）在 20 并发下达到约 **1240 QPS、p50 4 ms、p99 117 ms**；提升到 50 并发后，吞吐下降到 **671 QPS**，同时 p50/p95/p99 分别升至 24 ms / 348 ms / 510 ms。压测因此触发停止条件。

pprof 给出的最关键结论是：`diverseRank` 为当前 CPU 绝对热点，累计 CPU 占用达到 **82.86%**；同时 `runtime.mapaccess1_fast64` 占 25.21%，说明排序过程中存在大量 map 查找。报告将主要问题定位为 Feed 多样性排序的 **O(N²)** 计算。

结合 `develop` 分支源码进一步分析，可以确认当前 Feed 请求虽然已经使用 Redis ZSET 保存基础排序结果，但请求阶段仍然执行：

1. 全量读取候选；
2. 查询用户关注、Topic 关注、类型偏好、隐藏内容等用户画像；
3. 查询候选作者、类型、Topic；
4. 执行个性化打分；
5. 全量排序；
6. 执行 `diverseRank`；
7. 最后才进行分页。

因此，当前系统的核心问题不是单纯的硬件不足，而是：

> **把本应通过候选集截断、缓存和预计算解决的工作放到了 Feed 同步请求链路中。**

本报告建议采用渐进式优化，而不是立即引入复杂的大数据推荐架构。

### 核心优化路线

```text
当前：

全量 Candidate
      ↓
用户画像查询
      ↓
个性化打分
      ↓
全量 Sort
      ↓
O(N²) Diversity
      ↓
分页


优化后：

Global Top 200
      ↓
Redis User Profile
      ↓
Personalization
      ↓
Top 50
      ↓
轻量 Diversity
      ↓
Top 20
```

长期进一步演进为：

```text
内容变化
   ↓
异步 Feed Ranking
   ↓
Redis Feed Candidate
   ↓
用户请求
   ↓
轻量个性化
   ↓
Diversity
   ↓
Feed Response
```

---

# 2. 当前压测结论

## 2.1 新服务器表现

| 并发 | QPS | p50 | p95 | p99 | 错误率 |
|---:|---:|---:|---:|---:|---:|
| 20 | 1240 | 4 ms | 90 ms | 117 ms | 0% |
| 50 | 671 | 24 ms | 348 ms | 510 ms | 0% |

20 → 50 并发后：

- 并发提升约 2.5 倍；
- QPS 从 1240 降到 671，下降约 46%；
- p50 从 4 ms 上升到 24 ms；
- p95 从 90 ms 上升到 348 ms；
- p99 从 117 ms 上升到 510 ms。

这说明系统在 20 并发附近已经逐渐进入 CPU 计算瓶颈区间。

## 2.2 旧服务器表现

旧服务器为 2 vCPU / 1.6 GB：

| 并发 | QPS | p50 | p95 | p99 |
|---:|---:|---:|---:|---:|
| 20 | ~104 | 88 ms | 802 ms | 1.07 s |
| 50 | ~93 | 269 ms | 2.10 s | 2.77 s |

压测期间旧服务器还出现内存耗尽、SSH 握手超时等现象。

因此旧服务器存在明显资源短板，不建议继续作为性能优化的主要参照对象。

---

# 3. 性能瓶颈定位

## 3.1 pprof CPU 热点

报告中的 CPU profile：

| 函数 | flat | cum |
|---|---:|---:|
| `diverseRank` | 20.55% | **82.86%** |
| `runtime.mapaccess1_fast64` | 25.21% | 60.20% |
| runtime/maps | ~24% | ~24% |
| `topicOverlap` | 1.38% | 1.44% |

最重要的指标是：

> `diverseRank` cumulative CPU 达到 82.86%。

因此当前最优先的优化对象是 Feed Diversity Ranking，而不是继续扩大服务器规格。

---

# 4. 当前 Feed 架构分析

## 4.1 当前核心链路

当前 `ListForYou → getFeedPage` 的处理方式大致为：

```text
HTTP Request
    ↓
ListForYou
    ↓
getFeedPage
    ↓
ensureFeedEngine
    ↓
Redis Global Feed ZSET
    ↓
读取全部候选
    ↓
读取用户画像
    ├── Following
    ├── Followed Topics
    ├── Type Preference
    ├── Hidden Notes
    └── Hide Count
    ↓
读取候选元数据
    ├── Author
    ├── Type
    └── Topics
    ↓
personalizedScore
    ↓
sort.Slice
    ↓
diverseRank
    ↓
位置分页
    ↓
GetByIDs
    ↓
buildFeedResponse
```

这条链路在当前数据规模下可以工作，但随着帖子数和用户数增长，计算量会随着候选规模快速增长。

---

# 5. 架构问题一：Redis 只缓存了基础排序

当前 Feed Engine 已经使用 Redis ZSET 保存基础分：

```text
Global Feed ZSET
      ↓
Base Score
```

这是正确方向。

但请求阶段仍然需要：

```text
Redis ZSET
   ↓
全量读取
   ↓
Go 内存
   ↓
用户个性化
   ↓
Sort
   ↓
Diversity
```

因此 Redis 目前更准确地说是：

> **Global Base Ranking Cache**

而不是完整的用户 Feed Cache。

## 优化方向

应该逐步变成：

```text
Global Ranking
      ↓
Top N Candidate
      ↓
User Profile
      ↓
Personalization
      ↓
Diversity
```

而不是每个请求重新处理全部候选。

---

# 6. 架构问题二：全量 Candidate 导致计算规模不可控

当前 `getFeedPage` 会从 Redis ZSET 获取全部候选，再进入个性化和排序。

当前只有约 600 篇笔记，所以这个问题尚未完全暴露。

但是当数据增长到：

```text
10 万帖子
100 万帖子
1000 万帖子
```

如果仍然：

```text
ALL candidates
    ↓
personalize
    ↓
sort
    ↓
diversity
```

计算量会快速失控。

## 优化建议

采用多阶段 Candidate Pipeline：

```text
全部内容
    ↓
Global Ranking
    ↓
Top 200~300
    ↓
Personalization
    ↓
Top 50~100
    ↓
Diversity
    ↓
Top 20
```

第一阶段建议先将 Candidate 上限控制在 **100~300** 范围，通过压测选择最终值。

不建议直接截断到 20，因为候选过少会明显影响推荐多样性。

---

# 7. 架构问题三：Diversity 使用 O(N²)

当前 `diverseRank` 对候选集合进行逐轮选择：

```text
第 1 个结果：扫描候选
第 2 个结果：再次扫描剩余候选
第 3 个结果：再次扫描剩余候选
...
```

因此候选数量 N 增长时，计算量约为：

```text
O(N²)
```

例如：

```text
N = 100
≈ 10,000 级别比较/计算

N = 1,000
≈ 1,000,000 级别比较/计算

N = 10,000
≈ 100,000,000 级别比较/计算
```

这与 pprof 中 `diverseRank` 的 CPU 占用高度吻合。

---

# 8. Diversity 优化方案

## 8.1 第一阶段：限制 Diversity 输入规模

推荐：

```text
Global Top 200
      ↓
Personalization
      ↓
Top 50
      ↓
Diversity
      ↓
20
```

这样 Diversity 不再面对全部候选。

即使 Diversity 内部暂时保持 O(N²)，N 也已经从全量数据压缩到几十个。

---

## 8.2 第二阶段：优化 Diversity 数据结构

建议维护：

```go
authorCount map[int64]int
topicCount  map[int64]int
typeCount   map[int8]int
```

每次选择候选时，根据：

```text
Base Score
+ Author Diversity
+ Topic Diversity
+ Type Diversity
```

计算最终分数。

同时减少无意义的 map lookup。

---

## 8.3 第三阶段：只给 Top-K 候选携带完整 Topic 信息

当前候选结构包含：

```go
type scoredFeedItem struct {
    ID        int64
    Score     float64
    BaseScore float64
    AuthorID  int64
    Type      int8
    Topics    []int64
}
```

随着候选规模增加，`Topics []int64` 会带来额外 slice 分配和内存访问。

建议：

```text
Candidate
 ├── ID
 ├── Score
 ├── AuthorID
 └── Type

Topic information
        ↓
只在进入 Top-K / Diversity 阶段时加载
```

减少热路径上的对象大小、slice 分配以及 cache miss。

---

# 9. 架构问题四：分页发生得太晚

当前流程本质是：

```text
全部候选
 ↓
个性化
 ↓
排序
 ↓
Diversity
 ↓
分页
```

例如第一页：

```text
计算 1000 个候选
→ 返回 20
```

第二页：

```text
重新计算 1000 个候选
→ 丢掉前 20
→ 返回 20
```

因此当前属于：

> **Result Pagination，而不是 Compute Pagination。**

这在小规模数据下换取了顺序稳定和“不重不漏”，是合理的阶段性方案；但随着数据量增长，需要逐渐迁移到 Candidate Pool / Feed Cache。

---

# 10. 架构问题五：用户画像查询处于热路径

当前 Feed 请求需要获取：

```text
Following
Followed Topics
Type Preference
Hidden Notes
Hide Counts
Block information
```

这些数据本质上属于：

> **User Feed Profile**

而不是每一次 Feed 请求都应该重新聚合的数据。

## 推荐缓存模型

Redis：

```text
user:{uid}:following
user:{uid}:topics
user:{uid}:type_pref
user:{uid}:hidden
user:{uid}:blocked
```

或者聚合：

```text
user:{uid}:feed_profile
```

Feed 请求只需要读取 Redis。

---

# 11. 架构问题六：Hide Count 不应该实时统计

当前：

```text
CountHidesByType(userID)
```

属于典型的 read-time aggregation。

推荐改成：

```text
用户点击“不感兴趣”
        ↓
更新 hide record
        ↓
更新用户 Type Preference
        ↓
Redis
```

之后：

```text
Feed Request
        ↓
读取 Redis Preference
```

这样把：

```text
每次读取都 Count
```

变成：

```text
写入时维护
```

---

# 12. 架构问题七：Block 查询需要批量化

当前逻辑按照作者逐个判断 Block 状态。

如果候选中存在大量作者，可能形成：

```text
N authors
   ↓
N 次 Block 查询
```

无论底层是 MySQL 还是 Redis，都不应该让 Feed 热路径出现这种 N 次远程调用。

推荐：

```text
author IDs
    ↓
Redis Set / SMISMEMBER / MGET
    ↓
一次获取
```

或者维护用户级：

```text
user:{uid}:blocked_authors
```

直接完成本地集合判断。

---

# 13. 架构问题八：Note 数据还可以进一步缓存

Feed Ranking 得到：

```text
note IDs
```

之后仍然需要：

```text
repo.GetByIDs()
```

随着 QPS 增长，MySQL 会逐渐成为第二阶段瓶颈。

推荐：

```text
Feed ZSET
   ↓
Note IDs
   ↓
Redis Note Cache
   ↓
Cache Miss
   ↓
MySQL
```

MySQL 继续作为 Source of Truth。

---

# 14. 推荐目标架构

## 14.1 第一阶段架构

```text
                 MySQL
                   │
             Global Ranking
                   │
                   ↓
              Redis ZSET
                   │
              Top 200~300
                   │
                   ↓
          Redis User Profile
                   │
                   ↓
           Personalization
                   │
                Top 50
                   │
                   ↓
             Diversity
                   │
                Top 20
                   │
                   ↓
              Note Cache
                   │
                   ↓
                Response
```

---

# 15. 长期架构：异步 Feed Ranking

当用户和帖子规模进一步增长后，可以引入事件驱动：

```text
              MySQL
                 │
              Outbox
                 │
                 ↓
          Feed Worker / Queue
                 │
        ┌────────┴─────────┐
        ↓                  ↓
 Global Ranking       User Feed Update
        │                  │
        └────────┬─────────┘
                 ↓
               Redis
                 │
                 ↓
            Feed Service
                 │
                 ↓
        Lightweight Ranking
                 │
                 ↓
             Diversity
                 │
                 ↓
               Feed
```

当前项目已经具备 `outbox / queue / events / worker` 等模块，因此可以在现有架构上渐进演进，而不是推倒重来。

---

# 16. 不建议当前阶段做的事情

当前数据规模约：

- 用户：1033
- 帖子：612
- 评论：8648
- Topic：11
- 搜索索引：590

因此目前不建议直接引入：

- Kafka
- Flink
- 独立推荐服务
- Feature Store
- Elasticsearch 替代 Feed Ranking
- 大规模微服务拆分

原因是：

> 当前主要问题是算法复杂度和请求热路径设计，而不是系统组件数量不足。

先把单体 Go Feed Service 的计算效率提升，再考虑分布式推荐架构。

---

# 17. 优化优先级

| 优先级 | 优化项 | 收益 | 实施难度 |
|---|---|---|---|
| P0 | `diverseRank` 限制 Top-K | 极高 | 低 |
| P0 | Candidate 限制 100~300 | 极高 | 低 |
| P0 | Diversity 算法优化 | 极高 | 中 |
| P0 | 用户画像 Redis 化 | 高 | 中 |
| P1 | Block 批量判断 | 高 | 低 |
| P1 | Note Cache | 高 | 中 |
| P1 | 修复 worker 统计 SQL | 中 | 低 |
| P2 | User Feed Cache | 极高 | 中 |
| P2 | Feed Worker 异步预计算 | 高 | 中高 |
| P3 | Push/Pull Hybrid Feed | 极高 | 高 |
| P3 | Kafka/Flink 等大数据架构 | 长期 | 很高 |

---

# 18. 具体实施计划

## Sprint 1：消除 CPU 热点

### 修改 1：Candidate Limit

```go
const FeedCandidateLimit = 200
```

建议先从 200 开始。

压测：

```text
100
200
300
500
```

比较推荐质量和性能。

### 修改 2：Diversity 输入 Top 50

```text
Global Top 200
 ↓
Personalization
 ↓
Top 50
 ↓
Diversity
 ↓
20
```

### 修改 3：减少 map lookup

重点检查：

```text
author map
topic map
type map
following map
hidden map
```

减少重复 lookup，并尽可能提前过滤。

---

# 19. Sprint 2：Redis User Profile

建立：

```text
user:{uid}:following
user:{uid}:topics
user:{uid}:type_pref
user:{uid}:hidden
user:{uid}:blocked
```

所有写操作负责维护缓存。

Feed 请求：

```text
MySQL User Profile
        ↓
Redis
```

改为：

```text
Redis User Profile
        ↓
Feed Ranking
```

---

# 20. Sprint 3：Feed / Note Cache

建立：

```text
feed:user:{uid}
note:{id}
```

其中：

```text
feed:user:{uid}
```

可以保存：

```text
score → noteID
```

Note Cache 保存：

```text
note ID → Note JSON / Hash
```

Feed 请求尽量变成：

```text
Redis
 ↓
Redis
 ↓
Go lightweight ranking
 ↓
Response
```

MySQL 只处理 cache miss 和持久化业务操作。

---

# 21. Sprint 4：异步 Feed Engine

利用已有 Outbox / Queue / Worker：

```text
Note Published
      ↓
Outbox
      ↓
Queue
      ↓
Feed Worker
      ↓
Update Global Ranking
      ↓
Optional User Feed
```

对于当前规模：

```text
Global Feed + Read-time Personalization
```

已经足够。

规模进一步增长后：

```text
Push Feed
+
Pull Feed
```

再进行 Hybrid Merge。

---

# 22. 监控指标

下一轮压测建议增加 Feed 内部耗时指标：

```text
feed_total_ms
feed_candidate_fetch_ms
feed_profile_ms
feed_personalization_ms
feed_sort_ms
feed_diversity_ms
feed_note_db_ms
feed_response_ms
```

同时监控：

```text
CPU
RSS
Heap
GC Pause
Allocations
Redis QPS
Redis latency
MySQL QPS
MySQL latency
DB connection pool
```

这样下一轮可以直接回答：

```text
到底是谁慢？
```

而不是只依赖整体 p99 和 pprof。

---

# 23. 优化后的验收标准

建议下一轮压测使用相同的：

- 数据集
- 压测模型
- 80% Read / 20% Write
- 并发梯度
- 熔断标准

进行 Before / After 对比。

重点目标：

| 指标 | 当前 | 第一阶段目标 |
|---|---:|---:|
| 20 并发 QPS | 1240 | ≥ 1500 |
| 50 并发 QPS | 671 | ≥ 1200 |
| 50 并发 p99 | 510 ms | < 300 ms |
| Diversity CPU | 82.86% cum | < 30% |
| Feed MySQL profile queries | 多次 | 接近 0 |
| Candidate | 全量 | ≤ 200~300 |
| Diversity input | 全量 | ≤ 50~100 |

以上目标属于工程验收目标，不是当前代码已经达到的实测结果，需要通过优化后的实际压测验证。

---

# 24. 风险与注意事项

## 24.1 Candidate 截断可能影响推荐质量

不能只追求 QPS。

需要同时验证：

```text
作者多样性
Topic 多样性
内容类型多样性
点击率
停留时间
```

当前阶段没有足够的线上推荐指标，因此 Candidate Limit 应通过离线/压测数据逐步确定。

## 24.2 Feed Cache 需要处理失效

例如：

```text
Note 删除
Note 下架
用户拉黑
用户隐藏
作者被拉黑
```

必须能够快速从 Feed 中过滤。

因此 Feed Cache 不能替代最终权限/状态校验。

## 24.3 排序一致性

从当前源码设计来看，使用位置游标是为了保证个性化排序后的稳定顺序。

未来改为分数游标时，需要重新验证：

- 不重复；
- 不漏内容；
- 新内容插入；
- 分数变化；
- 用户偏好变化。

---

# 25. 最终架构结论

Xenflow 当前 Feed 系统已经完成了基础的：

```text
MySQL
+
Redis
+
Global Ranking
+
Personalization
+
Diversity
+
Cursor
```

整体方向是正确的。

真正限制系统扩展能力的是：

```text
全量 Candidate
       ↓
读时个性化
       ↓
全量 Sort
       ↓
O(N²) Diversity
       ↓
最后分页
```

这使得每一个 `/feed` 请求都携带较高的 CPU 计算成本。

因此最核心的架构优化原则是：

> **减少每次请求需要计算的 Candidate 数量，把用户画像从 MySQL 热路径迁移到 Redis，并逐渐将 Feed Ranking 从同步请求计算转向异步预计算。**

推荐最终演进路线：

```text
阶段 1
全量 Feed
 ↓
Top 200
 ↓
Top 50 Diversity
 ↓
20


阶段 2
Top 200
 ↓
Redis User Profile
 ↓
Top 50
 ↓
Diversity
 ↓
Note Cache


阶段 3
Global Ranking
 ↓
User Feed Cache
 ↓
Lightweight Personalization
 ↓
Diversity


阶段 4
Event / Outbox
 ↓
Feed Worker
 ↓
Push + Pull Hybrid Feed
 ↓
Redis
 ↓
Feed API
```

## 最重要的四项改动

```text
1. diverseRank：O(N²) → Top-K / 轻量算法
2. Candidate：全量 → 100~300
3. User Profile：MySQL read-time query → Redis
4. Feed：request-time computation → cache / async pre-computation
```

如果只做一件事：

> **先改 `getFeedPage + diverseRank`，把“全量候选 → 全量个性化 → O(N²) Diversity → 分页”改成“Top 200 → 个性化 → Top 50 → Diversity → 20”。**

这一步最可能直接改变当前 50 并发下 QPS 从 671 掉到平台期的性能表现。

---

## 附录：当前压测核心数据

来源于 Xenflow 压测报告：

- 新服务器：8 vCPU / 15 GB；
- 旧服务器：2 vCPU / 1.6 GB；
- 新服务器 20 并发：约 1240 QPS、p50 4 ms、p99 117 ms；
- 新服务器 50 并发：约 671 QPS、p50 24 ms、p95 348 ms、p99 510 ms；
- `diverseRank` cumulative CPU：82.86%；
- `runtime.mapaccess1_fast64`：25.21%；
- Feed `getFeedPage` heap cumulative：46.24%；
- 旧服务器压测出现内存耗尽及 SSH 超时；
- 当前数据规模：用户 1033、帖子 612、评论 8648、Topic 11、搜索索引 590。
