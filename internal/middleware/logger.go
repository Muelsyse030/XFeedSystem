package middleware

import (
    "time"
    "XFeedSystem/internal/pkg/logger"
    "github.com/gin-gonic/gin"
    "go.uber.org/zap"
)

func LoggerMiddleware() gin.HandlerFunc {
    return func(c *gin.Context) {
        start := time.Now()
        path := c.Request.URL.Path

        c.Next()  

        latency := time.Since(start)
        status := c.Writer.Status()

        logger.Logger.Info("request",
            zap.String("method", c.Request.Method),
            zap.String("path", path),
            zap.Int("status", status),
            zap.Duration("latency", latency),
            zap.String("client_ip", c.ClientIP()),
        )
    }
}