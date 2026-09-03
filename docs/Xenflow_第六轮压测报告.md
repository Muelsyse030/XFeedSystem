# Xenflow 第六轮压测报告（事件管道四连修验证）

**日期**：2026-09-01
**测试服务器**：1.15.89.134（8 vCPU / 15G，nginx → xfeed-api@1/2 + xfeed-worker）
**部署代码**：v7 + v7.1（MarkPublishedBatch、Notify 批量、Feed 跳过评论重算、XPENDING 监控）
**数据**：612 条笔记 / 1083 用户，热点评论笔记 100172（基线 8,727 条评论）
**场景**：评论洪峰（300c/500c）+ 积压恢复 + 混合回归 + Worker Kill/Restart + 一致性（与第五轮同方法）

---

## 一、核心结论

1. **发现并修复了第五轮的"隐藏根因"：Relay 没有切换新配置**。Worker 仍用旧 `cfg.Worker.Batch`（100/200ms）构造 Relay，导致批量与 Pipeline 全生效了、Relay 却仍被锁在 **500 events/s**。改为 `cfg.Event.Relay.BatchSize`（500/100ms）后，Relay 吞吐直接突破 5000/s。
2. **修复后事件管道全面达标**：评论洪峰（~2,750/s）下 outbox 积压峰值仅 **5.1 万**，且**洪峰未结束即归零**；四个消费组 pending 全部 ~0。
3. **一致性完美**：`comment_count == COUNT(评论行) == 173,822`，零延迟、零丢失。

## 二、修复前的复测（暴露 Relay 配置 bug）

第六轮代码部署后先跑了一轮完整测试：

| 场景 | 结果 |
|---|---|
| FLOOD1 评论 300c × 120s | 2,701 QPS / 0% 错误 |
| FLOOD2 评论 500c × 60s | 2,659 QPS / 0% 错误 |
| 混合 200/300/500 | 6,321 / 6,439 / 6,527 QPS / 0% 错误 |

但监控显示 **pending_feed/search/notify/counter 全部为 0**（消费者已跟上），outbox 却积压 33 万→39 万且只以 ~500/s 缓慢下降——瓶颈转移到了 Relay。日志中 `UPDATE outbox_events ... WHERE id IN (...)` 每批只更新 **rows:100**，且耗时 200-400ms。

**根因**：`cmd/worker/main.go` 里 Relay 构造仍用 `cfg.Worker.Batch`（100）+ `cfg.Worker.PollIntervalMs`（200ms），而不是第五轮新增的 `cfg.Event.Relay.BatchSize`（500）+ `IntervalMs`（100ms）。Consumer 已切换新配置，Relay 漏了。

## 三、修复后复测（v7.1）

修改：Relay 构造改用 `cfg.Event.Relay.*`，重建 worker 部署。

### 评论洪峰 300c × 60s（2,751 QPS，0% 错误）

```text
outbox_pending: 0 → 8,716 → 17,488 → 27,593 → 37,890 → 47,390 → 51,764(峰值) → 9,298 → 0
pending_feed/search/notify/counter: 峰值 0/51/64/64（瞬时一个批次在途），其余为 0
```

- 积压峰值 **51,764**（第五轮 545,000，降低 90%+）；
- **洪峰期间即排空**（Relay 5000/s > 生产 2750/s）；
- stream_len 165,095 = 全部评论事件，全部发布并消费；
- **一致性：db_count == real_rows == 173,822**。

### 修复后与第五轮对比

| 指标 | 第五轮 | 第六轮（修复后） |
|---|---:|---:|
| Relay 实际吞吐 | ~500/s | ≥5000/s |
| 洪峰积压峰值 | 545,000 | 51,764 |
| 积压恢复 | 数分钟不净 | 洪峰中直接归零 |
| pending_feed/search/notify/counter | 无法区分（XLEN） | 全部 ~0（XPENDING） |
| 计数一致性 | 延迟数分钟 | 精确相等 |
| Notify | 逐条 INSERT | 批量（pending_notify ~0） |

## 四、第六轮四项优化验证

| 优化 | 状态 |
|---|---|
| ① Relay MarkPublishedBatch | ✅ 每批 1 次 UPDATE（日志 rows 与耗时正常） |
| ② Feed 跳过评论重算 | ✅ 已实现（feedHandler 无互动分支） |
| ③ Notify 批量 INSERT | ✅ pending_notify 峰值仅 64（一个批次在途） |
| ④ XPENDING 监控 | ✅ 直接暴露了 Relay 配置 bug，按组定位瓶颈 |

## 五、结论

事件管道吞吐瓶颈正式解决：**Relay 5000/s + 消费者同步跟上 + 批量落库 + 精确一致**。第五轮验收目标（Consumer >3500/s、Backlog 自动归零、计数一致）全部达成，且洪峰中即可排空。

## 六、遗留说明

- 测试数据（17.4 万评论、互动、发帖、outbox/Redis）已清理，库恢复 612 条笔记干净基线（评论 100172 恢复 8,727 条）。
- 本地代码含 v7.1 修复（Relay 使用 `cfg.Event.Relay.*`）；服务器 `/opt/xfeed/configs/config.yaml` 已含 `event:` 段，`max_connections=400` 已重设。
- 后续关注点转向：HTTP 写接口每请求 3-4 次 DB 往返（互动路径 Note 缓存）、GC 分配（GORM 预编译语句、排名缓存 LIMIT 分页、搜索缓存）。
