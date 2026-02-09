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
