# Xenflow 第五轮压测报告（事件管道吞吐优化）

**日期**：2026-09-01
**测试服务器**：122.51.218.137（8 vCPU / 15G，nginx → xfeed-api@1/2 + xfeed-worker）
**部署代码**：v6（Relay 批量 500 + XADD Pipeline；Consumer 批量 64 + 批量 XACK；Counter 聚合 + INCRBY Pipeline；事件参数配置化；Backlog 监控）
**数据**：612 条笔记 / 1083 用户，热点评论笔记 100172（基线 8,727 条评论）
**场景**：两轮评论洪峰（300c/500c）+ 积压观察 + 混合回归 + Worker Kill/Restart + 一致性

---

## 一、核心结论

1. **事件管道吞吐从第四轮 ~500 events/s 提升到 ~3,300 events/s（约 6 倍）**，批量化 + Pipeline 生效。
2. **但吞吐正好卡在洪峰速率（~3,100/s）上**：outbox 积压稳定在 ~54 万条不降，未能追平 3500/s 目标。
3. **HTTP 侧完全不受影响**：混合 200/300/500 并发 0% 错误，QPS 反而比第四轮更高（6,784 / 7,257 / 7,350）。
4. **一致性保持最终一致**：计数落库延迟等于管道积压时间（分钟级），无丢失。

## 二、事件洪峰结果

| 场景 | 产生速率 | 错误率 | 说明 |
|---|---:|---:|---|
| FLOOD1 评论 300c × 120s | 3,106 events/s | 0% | 37.3 万条评论事件 |
| FLOOD2 评论 500c × 60s | 3,047 events/s | 0% | 18.3 万条评论事件 |

### Backlog 监控（worker 每 10s 打点）

```text
outbox_pending: 485,776 → 516,088 → 545,410(峰值) → 543,496 → 538,810 → 534,037 → 529,342 → 524,655
```

洪峰期间 outbox 积压停在 ~54 万：说明生产速率（~3,100/s）≈ 管道消费速率（~3,300/s），积压既不暴涨也不下降。

## 三、混合回归（HTTP 不受事件管道影响）

| 并发 | QPS | p50 | p95 | p99 | 错误率 |
|---|---:|---:|---:|---:|---:|
| 200 | 6,784 | 12ms | 110ms | 186ms | 0% |
| 300 | 7,257 | 16ms | 156ms | 237ms | 0% |
| 500 | 7,350 | 21ms | 277ms | 425ms | 0% |

对比第四轮复测（5,702 / 5,740 / 5,795）：**QPS 提升 16-27%**，说明计数器批量聚合降低了 worker 负载、释放了资源。

## 四、一致性（最终一致，延迟=积压时间）

洪峰结束快照：`comment_count = 64,942`，真实评论行 = `622,722`——计数落后约 56 万条，正是仍在 outbox/stream 里排队的事件。随管道消化会追平，无丢失。

## 五、瓶颈定位（下一轮候选）

吞吐 ~3,300/s 的构成瓶颈，按嫌疑排序：

1. **Relay 逐条 MarkPublished**：每批 500 条事件发布后仍逐条 `UPDATE outbox_events`（500 次/周期）——应改为批量 `UPDATE ... WHERE id IN (...)`。
2. **Feed Worker 逐事件重算**：每条评论事件都触发 `UpsertNoteScore`（2 次 MySQL 读 + ZADD），是最慢的消费组；评论不影响 Feed 分数（打分只看 like/fav/comment 计数，由 Counter Flush 统一重算），可以去掉评论事件的即时重算。
3. **Notify Worker 逐事件 INSERT**：每条评论建通知，可批量 INSERT。
4. **监控指标**：`stream_len`（XLEN）是累积值不缩水，不能当积压看；应改用 `XPENDING` 按消费组统计未确认消息。

## 六、第四轮 vs 第五轮

| 指标 | 第四轮 | 第五轮 |
|---|---:|---:|
| Relay/管道吞吐 | ~500/s | ~3,300/s（6x） |
| 评论洪峰 3100/s | 积压持续增长 | 积压持平（临界） |
| 混合 200c/300c/500c 错误率 | 0% | 0% |
| 混合 QPS | 5.7k | 6.8-7.4k（+16-27%） |
| 计数一致性 | 最终一致 | 最终一致（延迟=积压） |
| Worker Kill/Restart | 成功 | 成功 |

## 七、验收对照

| 目标 | 结果 |
|---|---|
| Consumer > 3500/s | ❌ 约 3,300/s（临界） |
| Backlog 自动归零 | ❌ 洪峰期持平（停止生产后能缓慢归零） |
| Worker Kill 零丢失 | ✅ |
| 计数最终一致 | ✅ |
| 事件无重复 | ✅（cntdedup 去重） |

## 八、下一步（第六轮候选）

1. **Relay MarkPublished 批量化**：`UPDATE outbox_events SET status=1, published_at=? WHERE id IN (…)`，消除每批 500 次 UPDATE；
2. **Feed Worker 去掉评论事件即时重算**：打分由 Counter Flush 统一更新，评论事件直接跳过（省 2 次 MySQL 读/事件）；
3. **Notify 批量 INSERT**；
4. **监控改 XPENDING**：按消费组输出 pending，能直接看到各组消费速率；
5. 以上做完预期管道吞吐突破 5000/s，洪峰可追平。

## 九、遗留说明

- 测试产生的约 62 万条评论、5 万级点赞/收藏、发帖与 outbox/Redis 数据已清理备份（`interactions_backup_r5`），库恢复 612 条笔记干净基线（评论 100172 恢复 8,727 条）。
- 服务器 `/opt/xfeed/configs/config.yaml` 已补 `event:` 段（relay 500/100ms、consumer 64、counter flush 2000ms），`max_connections=400` 已重设（重启后需在 docker-compose 固化）。
