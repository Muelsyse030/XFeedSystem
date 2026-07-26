package logger

import (
	"go.uber.org/zap"
    "go.uber.org/zap/zapcore"
)

var Logger *zap.Logger
var Sugar *zap.SugaredLogger

func Init(level string) {
	var cfg zap.Config
	cfg = zap.NewDevelopmentConfig()
	cfg.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	cfg.Level = zap.NewAtomicLevelAt(parseLevel(level))
	Logger, _ = cfg.Build(zap.AddCaller(), zap.AddCallerSkip(1))
    Sugar = Logger.Sugar()
}

func parseLevel(level string) zapcore.Level {
    switch level {
    case "debug":
        return zapcore.DebugLevel
    case "warn":
        return zapcore.WarnLevel
    case "error":
        return zapcore.ErrorLevel
    default:
        return zapcore.InfoLevel
    }
}

func Sync() {
    Logger.Sync()  
}