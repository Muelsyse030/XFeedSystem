package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"XFeedSystem/configs"
	"XFeedSystem/internal/pkg/config"
	"XFeedSystem/internal/routers"
	"XFeedSystem/internal/pkg/logger"
)

func main() {
	logger.Init("info")
	defer logger.Sync()
	cfg, err := config.LoadConfig()
	if err != nil {
		logger.Sugar.Fatalf("加载配置失败: %v", err)
	}

	logger.Sugar.Infof("服务器启动于端口 %d", cfg.Server.Port)

	db := configs.InitDB(cfg.MySQL.DSN)
	r := routers.SetupRouter(db, *cfg)

	addr := fmt.Sprintf(":%d", cfg.Server.Port)
	srv := &http.Server{
		Addr:    addr,
		Handler: r,
	}

	// 在 goroutine 中启动服务
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Sugar.Fatalf("服务器启动失败: %v", err)
		}
	}()

	// 等待中断信号
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	sig := <-quit
	logger.Sugar.Infof("收到信号 %v，开始优雅关闭...", sig)

	// 设置 30 秒超时进行优雅关闭
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		logger.Sugar.Fatalf("服务关闭异常: %v", err)
	}

	// 关闭数据库连接
	sqlDB, err := db.DB()
	if err == nil {
		sqlDB.Close()
	}

	logger.Sugar.Info("服务器已安全关闭")
}
