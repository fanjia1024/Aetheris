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

package metrics

import (
	"io"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/expfmt"
)

// 全局 Registry，供 API/Worker 注册与暴露
var DefaultRegistry = prometheus.NewRegistry()

func init() {
	DefaultRegistry.MustRegister(
		JobDuration, JobTotal, JobFailTotal,
		ToolDuration, LLMTokensTotal,
		WorkerBusy,
		QueueBacklog, StuckJobCount,
		// 2.0 Rate limiting metrics
		RateLimitWaitSeconds, RateLimitRejectionsTotal,
		ToolConcurrentGauge, LLMConcurrentGauge,
		JobParkedDuration,
		// 3.0-M4 Advanced metrics
		DecisionQualityScore, AnomalyDetectedTotal, SignatureVerificationTotal,
	)
}

// JobDuration Job 执行耗时（秒）
var JobDuration = prometheus.NewHistogramVec(
	prometheus.HistogramOpts{
		Name:    "aetheris_job_duration_seconds",
		Help:    "Job 执行耗时（秒）",
		Buckets: prometheus.DefBuckets,
	},
	[]string{"agent_id"},
)

// JobTotal Job 总数（按状态）
var JobTotal = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "aetheris_job_total",
		Help: "Job 总数（按状态）",
	},
	[]string{"status"}, // completed | failed | cancelled
)

// JobFailTotal Job 失败/取消总数（与 JobTotal 配合可算 job_fail_rate）
var JobFailTotal = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "aetheris_job_fail_total",
		Help: "Job 失败/取消总数",
	},
	[]string{"status"}, // failed | cancelled
)

// ToolDuration 工具调用耗时（秒）
var ToolDuration = prometheus.NewHistogramVec(
	prometheus.HistogramOpts{
		Name:    "aetheris_tool_duration_seconds",
		Help:    "工具调用耗时（秒）",
		Buckets: prometheus.DefBuckets,
	},
	[]string{"tool"},
)

// LLMTokensTotal LLM 调用 token 数
var LLMTokensTotal = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "aetheris_llm_tokens_total",
		Help: "LLM 调用 token 总数",
	},
	[]string{"direction"}, // input | output
)

// WorkerBusy 当前正在执行的 Job 数（每 Worker）
var WorkerBusy = prometheus.NewGaugeVec(
	prometheus.GaugeOpts{
		Name: "aetheris_worker_busy",
		Help: "当前正在执行的 Job 数",
	},
	[]string{"worker_id"},
)

// QueueBacklog 按队列的 Pending Job 积压数（2.0 可观测性）
var QueueBacklog = prometheus.NewGaugeVec(
	prometheus.GaugeOpts{
		Name: "aetheris_queue_backlog",
		Help: "Pending Job 积压数（按 queue 或 default）",
	},
	[]string{"queue"},
)

// StuckJobCount 卡住 Job 数：status=Running 且 updated_at 超过阈值的数量（2.0 Stuck Job Detector）
var StuckJobCount = prometheus.NewGauge(
	prometheus.GaugeOpts{
		Name: "aetheris_stuck_job_count",
		Help: "卡住的 Job 数（Running 且超过阈值未更新）",
	},
)

// RateLimitWaitSeconds 限流等待时间（秒）
var RateLimitWaitSeconds = prometheus.NewHistogramVec(
	prometheus.HistogramOpts{
		Name:    "aetheris_rate_limit_wait_seconds",
		Help:    "限流等待时间（秒）",
		Buckets: []float64{0.001, 0.01, 0.1, 0.5, 1, 2, 5, 10},
	},
	[]string{"type", "name"}, // type: tool|llm|queue, name: tool_name|provider|queue_class
)

// RateLimitRejectionsTotal 限流拒绝次数
var RateLimitRejectionsTotal = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "aetheris_rate_limit_rejections_total",
		Help: "限流拒绝次数",
	},
	[]string{"type", "name"},
)

// ToolConcurrentGauge Tool 当前并发数
var ToolConcurrentGauge = prometheus.NewGaugeVec(
	prometheus.GaugeOpts{
		Name: "aetheris_tool_concurrent",
		Help: "Tool 当前并发数",
	},
	[]string{"tool"},
)

// LLMConcurrentGauge LLM Provider 当前并发数
var LLMConcurrentGauge = prometheus.NewGaugeVec(
	prometheus.GaugeOpts{
		Name: "aetheris_llm_concurrent",
		Help: "LLM Provider 当前并发数",
	},
	[]string{"provider"},
)

// JobParkedDuration Job 处于 parked 状态的时长（秒）
var JobParkedDuration = prometheus.NewHistogramVec(
	prometheus.HistogramOpts{
		Name:    "aetheris_job_parked_duration_seconds",
		Help:    "Job 处于 parked 状态的时长（秒）",
		Buckets: []float64{10, 60, 300, 600, 1800, 3600, 7200, 14400}, // 10s ~ 4h
	},
	[]string{"agent_id"},
)

// DecisionQualityScore 决策质量评分（3.0-M4）
var DecisionQualityScore = prometheus.NewHistogramVec(
	prometheus.HistogramOpts{
		Name:    "aetheris_decision_quality_score",
		Help:    "决策质量评分（0-100）",
		Buckets: []float64{0, 20, 40, 60, 80, 100},
	},
	[]string{"job_id", "step_id"},
)

// AnomalyDetectedTotal 检测到的异常决策数（3.0-M4）
var AnomalyDetectedTotal = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "aetheris_anomaly_detected_total",
		Help: "检测到的异常决策数",
	},
	[]string{"anomaly_type", "severity"},
)

// SignatureVerificationTotal 签名验证次数（3.0-M4）
var SignatureVerificationTotal = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "aetheris_signature_verification_total",
		Help: "签名验证次数",
	},
	[]string{"result"}, // success | failed
)

// WritePrometheus 将 Prometheus 文本格式写入 w（供 Hertz 等复用）
func WritePrometheus(w io.Writer) error {
	metrics, err := DefaultRegistry.Gather()
	if err != nil {
		return err
	}
	enc := expfmt.NewEncoder(w, expfmt.FmtText)
	for _, mf := range metrics {
		if err := enc.Encode(mf); err != nil {
			return err
		}
	}
	return nil
}
