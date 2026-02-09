// Copyright 2026 fanjia1024
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

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
