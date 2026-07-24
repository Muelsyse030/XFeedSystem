package main

import (
	"XFeedSystem/configs"
	"XFeedSystem/internal/model"
	"XFeedSystem/internal/pkg/config"
	"fmt"
	"math/rand"
	"time"
)

var titles = []string{
	"Go语言并发编程实践",
	"微服务架构设计指南",
	"Redis缓存最佳实践",
	"MySQL索引优化技巧",
	"Kubernetes入门教程",
	"分布式系统设计模式",
	"深入理解Docker容器",
	"RESTful API设计规范",
	"gRPC通信框架详解",
	"消息队列选型对比",
	"数据库分库分表策略",
	"服务网格Istio实践",
	"云原生应用开发指南",
	"DevOps自动化部署",
	"日志收集与分析系统",
	"监控告警体系搭建",
	"网络安全防护实战",
	"Linux性能调优方法",
	"Git工作流最佳实践",
	"代码重构技巧分享",
	"设计模式在Go中的应用",
	"前端React性能优化",
	"TypeScript类型体操",
	"Node.js后端开发笔记",
	"Python数据分析入门",
	"机器学习模型部署",
	"图数据库Neo4j应用",
	"消息推送系统设计",
	"高并发秒杀系统架构",
	"OAuth2.0认证流程",
	"JWT令牌安全实践",
	"WebSocket实时通信",
	"分布式事务解决方案",
	"限流算法对比分析",
	"负载均衡策略详解",
	"CDN加速原理与实践",
	"DNS解析过程详解",
	"HTTPS证书管理指南",
	"容器编排与调度",
	"Prometheus监控实战",
	"Grafana可视化面板",
	"ELK日志平台搭建",
	"CI/CD流水线设计",
	"测试驱动开发TDD",
	"领域驱动设计DDD",
	"事件驱动架构EDA",
	"CQRS命令查询分离",
	"事件溯源EventSourcing",
	"Saga分布式事务模式",
	"断路器模式CircuitBreaker",
	"API网关Kong实践",
	"Nginx反向代理配置",
	"数据库连接池优化",
	"零拷贝技术详解",
	"内存对齐与性能",
	"Go GC垃圾回收机制",
	"Channel通信原理解析",
	"goroutine调度模型",
	"sync包并发原语",
	"context包使用指南",
}

var contents = []string{
	"本文详细介绍了相关的核心概念和实践方法。通过实际案例分析，帮助读者深入理解技术原理。\n\n## 核心要点\n\n1. 理解基本概念是深入学习的基石\n2. 动手实践比理论更重要\n3. 遇到问题要善于查阅官方文档\n\n## 总结\n\n掌握这些知识后，可以在实际项目中灵活运用，提升系统性能和开发效率。",
	"最近在项目中遇到了一个有趣的问题，经过排查发现和底层实现有关。在这里记录一下解决过程。\n\n## 问题描述\n\n在生产环境中发现接口响应时间偶发性飙升，通过profiling发现瓶颈在于不当的内存分配。\n\n## 解决方案\n\n使用对象池(sync.Pool)复用高频分配的对象，配合预分配slice容量，将GC压力降低了60%。\n\n## 效果\n\nP99延迟从500ms降至50ms，QPS提升了3倍。",
	"这是我在学习新技术时整理的笔记，希望能帮助到正在学习的同学。\n\n学习路线：\n- 第一阶段：掌握基础语法和核心概念\n- 第二阶段：动手做一些小项目巩固\n- 第三阶段：阅读源码，理解底层实现\n- 第四阶段：参与开源项目，提升实战能力\n\n坚持学习，每天进步一点点。",
	"技术选型是软件开发中非常重要的决策环节。本文从多个维度对比了主流技术方案的优缺点。\n\n| 方案 | 性能 | 易用性 | 生态 | 社区活跃度 |\n|------|------|--------|------|------------|\n| 方案A | 高 | 中 | 丰富 | 活跃 |\n| 方案B | 中 | 高 | 一般 | 活跃 |\n| 方案C | 极高 | 低 | 丰富 | 一般 |\n\n综合考虑，推荐在中小型项目中使用方案A。",
	"安全性是每个开发者都应该重视的话题。本文总结了一些常见的安全漏洞和防护措施。\n\n### XSS防护\n- 对所有用户输入进行转义\n- 设置CSP头限制脚本来源\n- 使用HttpOnly Cookie\n\n### SQL注入防护\n- 使用参数化查询\n- 避免拼接SQL语句\n- 最小权限原则\n\n### CSRF防护\n- 使用CSRF Token\n- 验证Referer头\n- SameSite Cookie",
	"性能优化是一个系统性工程，需要从多个层面进行分析和改进。\n\n## 优化层次\n\n1. **代码层面**：算法优化、减少内存分配\n2. **数据库层面**：索引优化、查询优化、连接池\n3. **架构层面**：缓存、异步、削峰\n4. **基础设施层面**：CDN、负载均衡、扩容\n\n## 方法论\n\n遵循测量-分析-优化-验证的循环，避免过早优化。",
	"最近阅读了一本经典技术书籍，收获很大。分享一下读书笔记。\n\n> 软件架构的本质是管理复杂性。好的架构让系统易于理解、开发和维护。\n\n关键收获：\n1. 关注点分离是架构设计的核心原则\n2. 接口比实现更重要\n3. 好的抽象降低认知负担\n4. 过度设计和不设计一样糟糕",
	"项目上线过程中遇到一些坑，在此记录供大家参考。\n\n1. 数据库迁移要先在测试环境验证\n2. 配置变更要有回滚方案\n3. 监控告警要提前配置好\n4. 灰度发布可以降低风险\n5. 保持和运维团队的沟通\n\n希望这些经验能帮助大家少走弯路。",
	"开源项目贡献指南，写给想要参与开源但不知从何下手的同学。\n\n### 第一步：选择项目\n- 选择自己日常使用的项目\n- 关注good first issue标签\n- 从文档改进开始\n\n### 第二步：了解流程\n- 阅读CONTRIBUTING.md\n- Fork项目到自己的仓库\n- 创建功能分支进行开发\n\n### 第三步：提交PR\n- 遵循项目的代码风格\n- 编写清晰的commit message\n- 耐心等待review",
	"面试中常考的数据结构与算法题目整理。\n\n### 数组与链表\n- 反转链表（迭代/递归）\n- 环形链表检测\n- 合并两个有序链表\n\n### 树与图\n- 二叉树遍历（前中后序、层序）\n- 最近公共祖先\n- 最短路径算法\n\n### 动态规划\n- 爬楼梯问题\n- 背包问题变体\n- 最长公共子序列\n\n建议先理解思想，再刷题巩固。",
}

var authorIDs = []int64{1, 2, 3, 4, 5}

func main() {
	cfg, err := config.LoadConfig()
	if err != nil {
		panic("load config: " + err.Error())
	}

	db := configs.InitDB(cfg.MySQL.DSN)

	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	now := time.Now()
	total := 150

	for i := 0; i < total; i++ {
		daysAgo := rng.Intn(90) // spread across last 90 days
		hoursAgo := rng.Intn(24)
		minutesAgo := rng.Intn(60)

		publishedAt := now.AddDate(0, 0, -daysAgo).
			Add(-time.Duration(hoursAgo) * time.Hour).
			Add(-time.Duration(minutesAgo) * time.Minute)

		note := model.Note{
			AuthorID:    authorIDs[rng.Intn(len(authorIDs))],
			Title:       titles[rng.Intn(len(titles))],
			Content:     contents[rng.Intn(len(contents))],
			Status:      model.NoteStatusPublished,
			Type:        1,
			PublishedAt: publishedAt,
			LikeCount:   int64(rng.Intn(200)),
			FavoriteCount: int64(rng.Intn(50)),
			CommentCount:  int64(rng.Intn(30)),
		}

		if err := db.Create(&note).Error; err != nil {
			panic(fmt.Sprintf("insert note %d: %v", i+1, err))
		}

		if (i+1)%25 == 0 {
			fmt.Printf("Inserted %d/%d notes...\n", i+1, total)
		}
	}

	fmt.Printf("Done! Successfully inserted %d notes into the database.\n", total)
}
