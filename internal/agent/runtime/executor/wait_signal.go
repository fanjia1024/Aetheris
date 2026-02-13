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

import "errors"

// SignalWaitRequired 表示节点需要等待外部 signal 后再继续；Runner 应写入 job_waiting 并返回 ErrJobWaiting。
type SignalWaitRequired struct {
	CorrelationKey string
	Reason         string
}

func (e *SignalWaitRequired) Error() string {
	reason := e.Reason
	if reason == "" {
		reason = "signal_wait"
	}
	return "signal wait required: " + reason + " (" + e.CorrelationKey + ")"
}

func signalWaitFromError(runErr error) (correlationKey string, reason string, ok bool) {
	if runErr == nil {
		return "", "", false
	}
	var capReq *CapabilityRequiresApproval
	if errors.As(runErr, &capReq) && capReq != nil && capReq.CorrelationKey != "" {
		return capReq.CorrelationKey, "capability_approval", true
	}
	var sw *SignalWaitRequired
	if errors.As(runErr, &sw) && sw != nil && sw.CorrelationKey != "" {
		r := sw.Reason
		if r == "" {
			r = "signal_wait"
		}
		return sw.CorrelationKey, r, true
	}
	return "", "", false
}
