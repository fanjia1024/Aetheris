// Package utils 通用小工具，不依赖 internal（设计 struct.md 4）
package utils

// CoalesceString 返回第一个非空字符串
func CoalesceString(ss ...string) string {
	for _, s := range ss {
		if s != "" {
			return s
		}
	}
	return ""
}

// DefaultInt 若 v 为 0 则返回 defaultVal
func DefaultInt(v, defaultVal int) int {
	if v == 0 {
		return defaultVal
	}
	return v
}
