// Package errors 提供统一错误辅助，不依赖 internal（设计 struct.md 4）
package errors

import (
	"errors"
	"fmt"
)

// 常用哨兵错误（可按需扩展错误码）
var (
	ErrNotFound   = errors.New("not found")
	ErrInvalidArg = errors.New("invalid argument")
)

// Wrap 包装错误并附加消息
func Wrap(err error, msg string) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", msg, err)
}

// Wrapf 带格式的 Wrap
func Wrapf(err error, format string, args ...interface{}) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", fmt.Sprintf(format, args...), err)
}
