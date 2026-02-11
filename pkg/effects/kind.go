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

// Package effects provides Effect isolation layer for deterministic execution.
// All non-deterministic operations (LLM, Tool, HTTP, Time, Random) must go through
// this package to ensure replayability and idempotency.
package effects

// Kind represents the type of side effect.
type Kind string

const (
	KindLLM    Kind = "llm"
	KindTool   Kind = "tool"
	KindHTTP   Kind = "http"
	KindTime   Kind = "time"
	KindRandom Kind = "random"
	KindSleep  Kind = "sleep"
)

// String returns the string representation of Kind.
func (k Kind) String() string {
	return string(k)
}
