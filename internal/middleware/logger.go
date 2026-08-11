package middleware

import (
	"XFeedSystem/internal/pkg/logger"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"time"
)

func LoggerMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path

		c.Next()

		latency := time.Since(start)
		status := c.Writer.Status()

		// 生产环境只记录慢请求和错误，避免每请求日志的系统调用与格式化开销
		if status >= 400 || latency > 500*time.Millisecond {
			logger.Logger.Info("slow_or_error_request",
				zap.String("method", c.Request.Method),
				zap.String("path", path),
				zap.Int("status", status),
				zap.Duration("latency", latency),
				zap.String("client_ip", c.ClientIP()),
			)
		}
	}
}
