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
	)
}

// JobDuration Job 执行耗时（秒）
var JobDuration = prometheus.NewHistogramVec(
	prometheus.HistogramOpts{
		Name:    "corag_job_duration_seconds",
		Help:    "Job 执行耗时（秒）",
		Buckets: prometheus.DefBuckets,
	},
	[]string{"agent_id"},
)

// JobTotal Job 总数（按状态）
var JobTotal = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "corag_job_total",
		Help: "Job 总数（按状态）",
	},
	[]string{"status"}, // completed | failed | cancelled
)

// JobFailTotal Job 失败/取消总数（与 JobTotal 配合可算 job_fail_rate）
var JobFailTotal = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "corag_job_fail_total",
		Help: "Job 失败/取消总数",
	},
	[]string{"status"}, // failed | cancelled
)

// ToolDuration 工具调用耗时（秒）
var ToolDuration = prometheus.NewHistogramVec(
	prometheus.HistogramOpts{
		Name:    "corag_tool_duration_seconds",
		Help:    "工具调用耗时（秒）",
		Buckets: prometheus.DefBuckets,
	},
	[]string{"tool"},
)

// LLMTokensTotal LLM 调用 token 数
var LLMTokensTotal = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "corag_llm_tokens_total",
		Help: "LLM 调用 token 总数",
	},
	[]string{"direction"}, // input | output
)

// WorkerBusy 当前正在执行的 Job 数（每 Worker）
var WorkerBusy = prometheus.NewGaugeVec(
	prometheus.GaugeOpts{
		Name: "corag_worker_busy",
		Help: "当前正在执行的 Job 数",
	},
	[]string{"worker_id"},
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
