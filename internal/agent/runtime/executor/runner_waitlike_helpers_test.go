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

package executor

import (
	"testing"

	"rag-platform/internal/agent/planner"
)

func TestIsWaitLikeNodeType(t *testing.T) {
	if !isWaitLikeNodeType(planner.NodeWait) {
		t.Fatal("planner.NodeWait should be wait-like")
	}
	if !isWaitLikeNodeType(planner.NodeApproval) {
		t.Fatal("planner.NodeApproval should be wait-like")
	}
	if !isWaitLikeNodeType(planner.NodeCondition) {
		t.Fatal("planner.NodeCondition should be wait-like")
	}
	if isWaitLikeNodeType(planner.NodeTool) {
		t.Fatal("planner.NodeTool should not be wait-like")
	}
}

func TestWaitDefaultsForNodeType(t *testing.T) {
	k, r := waitDefaultsForNodeType(planner.NodeApproval)
	if k != "signal" || r != "approval_required" {
		t.Fatalf("approval defaults = (%q,%q), want (signal,approval_required)", k, r)
	}
	k, r = waitDefaultsForNodeType(planner.NodeCondition)
	if k != planner.WaitKindCondition || r != "wait_condition" {
		t.Fatalf("condition defaults = (%q,%q), want (%q,wait_condition)", k, r, planner.WaitKindCondition)
	}
	k, r = waitDefaultsForNodeType(planner.NodeWait)
	if k != "signal" || r != "" {
		t.Fatalf("wait defaults = (%q,%q), want (%q,%q)", k, r, "signal", "")
	}
}
