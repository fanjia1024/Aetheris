package log

import (
	"log/slog"
	"os"
)

// Logger 简单封装，供 internal 使用
type Logger struct {
	*slog.Logger
}

// Config 日志配置（可与 config 包对接）
type Config struct {
	Level  string `mapstructure:"level"`
	Format string `mapstructure:"format"`
	File   string `mapstructure:"file"`
}

// NewLogger 根据配置创建 Logger，cfg 可为 nil 使用默认
func NewLogger(cfg *Config) (*Logger, error) {
	level := slog.LevelInfo
	if cfg != nil && cfg.Level != "" {
		switch cfg.Level {
		case "debug":
			level = slog.LevelDebug
		case "warn":
			level = slog.LevelWarn
		case "error":
			level = slog.LevelError
		}
	}
	opts := &slog.HandlerOptions{Level: level}
	var h slog.Handler = slog.NewJSONHandler(os.Stdout, opts)
	if cfg != nil && cfg.Format == "text" {
		h = slog.NewTextHandler(os.Stdout, opts)
	}
	return &Logger{Logger: slog.New(h)}, nil
}
