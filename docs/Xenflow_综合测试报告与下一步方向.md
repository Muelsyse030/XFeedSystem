# Xenflow 综合测试报告（101.43.11.57）与下一步优化方向

**日期**：2026-09-01
**测试服务器**：101.43.11.57（8 vCPU / 15G，nginx → xfeed-api@1/2 + xfeed-worker，v7.1 代码）
**数据**：612 条笔记 / 1083 用户（干净基线）
**范围**：全量混合梯度（20-1000c）+ 热点点赞/评论 + CPU/分配采样 + 一致性

---

## 一、混合场景（80/20，13 接口）

| 并发 | QPS | p50 | p95 | p99 | 最大 | 错误率 |
|---|---:|---:|---:|---:|---:|---:|
| 20 | 3,426 | 2ms | 16ms | 52ms | 79ms | 0% |
| 50 | 4,046 | 5ms | 36ms | 110ms | 217ms | 0% |
| 100 | 4,568 | 9ms | 69ms | 183ms | 364ms | 0% |
| 150 | 4,768 | 12ms | 111ms | 253ms | 601ms | 0% |
| 200 | 4,723 | 15ms | 159ms | 331ms | 704ms | 0% |
| 300 | 4,777 | 21ms | 247ms | 464ms | 1,130ms | 0% |
| 500 | 4,724 | 29ms | 434ms | 766ms | 1,933ms | 0% |
| 800 | 4,824 | 25ms | 770ms | 1,310ms | 3,493ms | 0% |
| 1000 | 4,564 | 34ms | 972ms | 1,664ms | 4,115ms | 0% |

**20-1000 并发全程 0% 错误**，吞吐平台在 ~4.7K QPS（150c 后基本不再增长，1000c 时 p99 1.66s）。系统已非常稳定，瓶颈从"高并发出错"转变为"每请求成本决定的吞吐上限"。

## 二、热点写场景（单笔记，0% 错误）

| 接口 | c100 | c200 | c300 |
|---|---:|---:|---:|
| 热点点赞（100140） | 2,884 QPS | 2,308 | 2,662 |
| 热点评论（100172） | 2,683 QPS | 2,565 | 2,717 |

热点写 QPS 稳定在 ~2.3-2.9K，受限于每请求 3-4 次 DB 往返（查笔记 + 拉黑校验 + 插入 + outbox）。

## 三、pprof：CPU 与分配

### CPU（300c / 800c）

| 函数 | 300c | 800c | 说明 |
|---|---:|---:|---|
| syscall.Syscall6 | 18.87% | 17.64% | I/O（网络/Redis/MySQL） |
| GC（mallocgc cum / scanobject / findObject 等） | ~15% | ~13% | 分配驱动的 GC |
| powf | 1.50% | 1.28% | Diversity 幂次 |

### 分配（500c，20s 采样 ~2.5GB）

| 来源 | 分配占比 | 说明 |
|---|---:|---|
| Feed 读链路（FeedHandler.List cum） | 48.16% | getFeedPage 31.95% + JSON + 排名缓存 |
| GORM/MySQL 驱动 | ~25% | AddVar 2.82M 次(12.6%) + clone/AddClause + reflect.New 2.48M 次 + readRow |
| Redis 客户端 | ~15% | readStringReply 8.84% + ZSliceCmd/zAddArgs（排名缓存全量读写） |
| HTTP gzip（flate） | 6.2% | 压测环境经 nginx gzip 的开销 |
| 其他 | - | diverseRank 66MB、BuildSummary 44MB、JSON 117MB |

结论与历轮一致：**I/O 第一、GC 第二**；分配由"Feed 读路径 + GORM 反射 + Redis ZSET 全量读写"驱动。

## 四、一致性

热点评论测试后快照：comment_count 落后真实行数约 3.8 万（测试刚结束、管道还在追平），属正常最终一致；点赞/收藏计数已对齐（+282 赞与新增点赞行一致）。

## 五、下一步优化方向（优先级）

当前系统状态：**稳定（0% 错误）但吞吐平台化（~4.7K）**，瓶颈 = 每请求 I/O 往返 + GC 分配。下一步按收益排序：

### P0：减少互动路径的 DB 往返（热点写 2.6K → 目标 4K+）

每次点赞/收藏/评论仍做 3-4 次 DB 往返（`GetByID` + 拉黑校验 + 插入 + outbox）。优化：
1. **互动路径用 Note 缓存**（`s.GetByID` 已带 Redis 缓存，替换 `repo.GetByID`），每次写请求省 1 次 MySQL 读；
2. **拉黑校验合并/缓存**：批量判断 + 用户级 blocked 集合（已有 `BlockedIDsKey` 缓存未用于写路径）。

### P1：GORM 预编译语句（分配 ~25% → 减半）

`gorm.Open(..., &gorm.Config{PrepareStmt: true})` 或 DSN `prepStmts=true`。消除 AddVar/clone/AddClause/reflect.New 这一整块（分配次数第一），同时减少 MySQL 端 SQL 解析。

### P2：排名缓存 LIMIT 分页 + 搜索缓存

- 排名缓存翻页改为 `ZRANGEBYSCORE ... LIMIT offset count` 直接取一页（现在全量取再切，readStringReply 8.8% + ZSliceCmd 4.3%）；
- `/search` 缓存 Meili 的 ID 结果 15-30s（搜索占 CPU cum 16%，权重只有 10%）。

### P3：小项

- `powf` 查表（1.5% CPU）；
- 压测环境去掉 gzip（flate 6.2% 是测试开销）；
- `GOGC=200~400`（15GB 内存富余，用空间换 GC）。

## 六、结论

系统已从"高并发崩溃/出错"走到"全程 0 错误、平台稳定"阶段。**下一次优化的主战场是"单请求成本"**：互动路径减 DB 往返（P0）是热点写和整体 QPS 最直接的杠杆；GORM 预编译（P1）和排名缓存分页（P2）针对 GC 与分配。预期组合落地后：混合吞吐可从 ~4.7K 冲击 6K+，热点写从 ~2.6K 冲击 4K+。
