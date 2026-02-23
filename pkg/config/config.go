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

package config

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/viper"
)

// Config 应用配置结构体
type Config struct {
	API             APIConfig             `mapstructure:"api"`
	Agent           AgentConfig           `mapstructure:"agent"`
	Runtime         RuntimeConfig         `mapstructure:"runtime"`
	JobStore        JobStoreConfig        `mapstructure:"jobstore"`
	EffectStore     EffectStoreConfig     `mapstructure:"effect_store"`
	CheckpointStore CheckpointStoreConfig `mapstructure:"checkpoint_store"`
	Worker          WorkerConfig          `mapstructure:"worker"`
	Model           ModelConfig           `mapstructure:"model"`
	Storage         StorageConfig         `mapstructure:"storage"`
	Log             LogConfig             `mapstructure:"log"`
	Monitoring      MonitoringConfig      `mapstructure:"monitoring"`
	RateLimits      RateLimitsConfig      `mapstructure:"rate_limits"`
}

// RuntimeConfig 运行时环境配置
type RuntimeConfig struct {
	Profile string `mapstructure:"profile"` // dev | prod
	Strict  bool   `mapstructure:"strict"`  // true 时启用生产强校验门禁
}

// RateLimitsConfig 限流配置（Tool + LLM）
type RateLimitsConfig struct {
	Tools map[string]ToolRateLimitConfig `mapstructure:"tools"`
	LLM   map[string]LLMRateLimitConfig  `mapstructure:"llm"`
}

// ToolRateLimitConfig 单个 Tool 的限流配置
type ToolRateLimitConfig struct {
	QPS           float64 `mapstructure:"qps"`
	MaxConcurrent int     `mapstructure:"max_concurrent"`
	Burst         int     `mapstructure:"burst"`
}

// LLMRateLimitConfig 单个 LLM Provider 的限流配置
type LLMRateLimitConfig struct {
	TokensPerMinute   int     `mapstructure:"tokens_per_minute"`
	RequestsPerMinute float64 `mapstructure:"requests_per_minute"`
	MaxConcurrent     int     `mapstructure:"max_concurrent"`
}

// JobStoreConfig 任务事件存储配置（事件流 + 租约）
type JobStoreConfig struct {
	Type          string `mapstructure:"type"`           // memory | postgres
	DSN           string `mapstructure:"dsn"`            // Postgres 连接串，type=postgres 时必填
	LeaseDuration string `mapstructure:"lease_duration"` // 租约时长，如 "30s"，空则默认 30s
}

// EffectStoreConfig 副作用存储配置
type EffectStoreConfig struct {
	Type string `mapstructure:"type"` // memory | postgres
	DSN  string `mapstructure:"dsn"`  // Postgres 连接串，type=postgres 时必填
}

// CheckpointStoreConfig Checkpoint 存储配置
type CheckpointStoreConfig struct {
	Type string `mapstructure:"type"` // memory | postgres
	DSN  string `mapstructure:"dsn"`  // Postgres 连接串，type=postgres 时必填
}

// AgentConfig Agent 与 Job 调度相关配置
type AgentConfig struct {
	JobScheduler JobSchedulerConfig `mapstructure:"job_scheduler"`
	ADK          AgentADKConfig     `mapstructure:"adk"` // Eino ADK 主 Runner（对话 run/resume/stream）
}

// AgentADKConfig ADK Runner 配置（主对话入口）
type AgentADKConfig struct {
	Enabled         *bool  `mapstructure:"enabled"`          // 为 false 时禁用 ADK，使用原 Plan→Execute Agent；未配置时默认 true
	CheckpointStore string `mapstructure:"checkpoint_store"` // memory | 留空；后续可扩展 postgres/redis
}

// JobSchedulerConfig Scheduler 并发、重试、backoff 与队列优先级
type JobSchedulerConfig struct {
	// Enabled 为 false 时 API 不启动进程内 Scheduler，由独立 Worker 进程拉取执行（分布式模式）；未配置时默认 true
	Enabled        *bool    `mapstructure:"enabled"`
	MaxConcurrency int      `mapstructure:"max_concurrency"` // 最大并发执行数，<=0 使用默认 2
	RetryMax       int      `mapstructure:"retry_max"`       // 失败后最大重试次数（不含首次），<0 使用默认 2
	Backoff        string   `mapstructure:"backoff"`         // 重试前等待时间，如 "1s"，空则默认 1s
	Queues         []string `mapstructure:"queues"`          // 按优先级轮询的队列列表，如 ["realtime","default","background"]；空则不区分队列
}

// APIConfig API 服务配置
type APIConfig struct {
	Port       int              `mapstructure:"port"`
	Host       string           `mapstructure:"host"`
	Timeout    string           `mapstructure:"timeout"`
	CORS       CORSConfig       `mapstructure:"cors"`
	Middleware MiddlewareConfig `mapstructure:"middleware"`
	Forensics  ForensicsConfig  `mapstructure:"forensics"`
	Grpc       GrpcConfig       `mapstructure:"grpc"`
}

// ForensicsConfig 取证查询类接口配置
type ForensicsConfig struct {
	Experimental bool `mapstructure:"experimental"`
}

// GrpcConfig gRPC 服务配置
type GrpcConfig struct {
	Enable bool `mapstructure:"enable"`
	Port   int  `mapstructure:"port"`
}

// CORSConfig CORS 配置
type CORSConfig struct {
	Enable       bool     `mapstructure:"enable"`
	AllowOrigins []string `mapstructure:"allow_origins"`
}

// MiddlewareConfig 中间件配置
type MiddlewareConfig struct {
	Auth          bool   `mapstructure:"auth"`
	RateLimit     bool   `mapstructure:"rate_limit"`
	RateLimitRPS  int    `mapstructure:"rate_limit_rps"`
	JWTKey        string `mapstructure:"jwt_key"`
	JWTTimeout    string `mapstructure:"jwt_timeout"`     // 如 "1h"
	JWTMaxRefresh string `mapstructure:"jwt_max_refresh"` // 如 "1h"
}

// WorkerConfig Worker 服务配置
type WorkerConfig struct {
	Concurrency  int      `mapstructure:"concurrency"`
	QueueSize    int      `mapstructure:"queue_size"`
	RetryCount   int      `mapstructure:"retry_count"`
	RetryDelay   string   `mapstructure:"retry_delay"`
	Timeout      string   `mapstructure:"timeout"`
	PollInterval string   `mapstructure:"poll_interval"` // Agent Job Claim 轮询间隔，如 "2s"
	MaxAttempts  int      `mapstructure:"max_attempts"`  // Agent Job 最大执行次数（含首次），达此后标记 Failed 不再调度；<=0 时默认 3
	Capabilities []string `mapstructure:"capabilities"`  // Worker 能力列表（如 llm, tool, rag）；Scheduler 仅派发 RequiredCapabilities 满足的 Job；空表示接受任意 Job
}

// ModelConfig 模型配置
type ModelConfig struct {
	LLM       LLMConfig       `mapstructure:"llm"`
	Embedding EmbeddingConfig `mapstructure:"embedding"`
	Vision    VisionConfig    `mapstructure:"vision"`
	Defaults  DefaultsConfig  `mapstructure:"defaults"`
}

// LLMConfig LLM 模型配置
type LLMConfig struct {
	Providers map[string]ProviderConfig `mapstructure:"providers"`
}

// EmbeddingConfig Embedding 模型配置
type EmbeddingConfig struct {
	Providers map[string]ProviderConfig `mapstructure:"providers"`
}

// VisionConfig Vision 模型配置
type VisionConfig struct {
	Providers map[string]ProviderConfig `mapstructure:"providers"`
}

// ProviderConfig 模型提供商配置
type ProviderConfig struct {
	APIKey  string               `mapstructure:"api_key"`
	BaseURL string               `mapstructure:"base_url"`
	Models  map[string]ModelInfo `mapstructure:"models"`
}

// ModelInfo 模型信息
type ModelInfo struct {
	Name          string  `mapstructure:"name"`
	ContextWindow int     `mapstructure:"context_window"`
	Temperature   float64 `mapstructure:"temperature"`
	Dimension     int     `mapstructure:"dimension"`
	InputLimit    int     `mapstructure:"input_limit"`
	MaxTokens     int     `mapstructure:"max_tokens"`
}

// DefaultsConfig 默认模型配置
type DefaultsConfig struct {
	LLM       string `mapstructure:"llm"`
	Embedding string `mapstructure:"embedding"`
	Vision    string `mapstructure:"vision"`
}

// StorageConfig 存储配置
type StorageConfig struct {
	Metadata MetadataConfig `mapstructure:"metadata"`
	Vector   VectorConfig   `mapstructure:"vector"`
	Object   ObjectConfig   `mapstructure:"object"`
	Cache    CacheConfig    `mapstructure:"cache"`
	Ingest   IngestConfig   `mapstructure:"ingest"`
}

// IngestConfig 入库管线配置（索引批大小、并发等）
type IngestConfig struct {
	BatchSize   int `mapstructure:"batch_size"`
	Concurrency int `mapstructure:"concurrency"`
}

// MetadataConfig 元数据存储配置
type MetadataConfig struct {
	Type     string `mapstructure:"type"`
	DSN      string `mapstructure:"dsn"`
	PoolSize int    `mapstructure:"pool_size"`
}

// VectorConfig 向量存储配置（memory 为内置内存；redis/milvus2/es8 等使用 eino-ext 对应组件）
type VectorConfig struct {
	Type       string `mapstructure:"type"`
	Addr       string `mapstructure:"addr"`
	DB         string `mapstructure:"db"`         // memory 忽略；Redis 为 DB 编号，如 "0"
	Collection string `mapstructure:"collection"` // 默认索引/集合名，ingest 与 query 共用
	Password   string `mapstructure:"password"`   // Redis 等后端密码，可选
}

// ObjectConfig 对象存储配置
type ObjectConfig struct {
	Type     string `mapstructure:"type"`
	Endpoint string `mapstructure:"endpoint"`
	Bucket   string `mapstructure:"bucket"`
	Region   string `mapstructure:"region"`
}

// CacheConfig 缓存配置
type CacheConfig struct {
	Type     string `mapstructure:"type"`
	Addr     string `mapstructure:"addr"`
	DB       int    `mapstructure:"db"`
	Password string `mapstructure:"password"`
}

// LogConfig 日志配置
type LogConfig struct {
	Level  string `mapstructure:"level"`
	Format string `mapstructure:"format"`
	File   string `mapstructure:"file"`
}

// MonitoringConfig 监控配置
type MonitoringConfig struct {
	Prometheus PrometheusConfig `mapstructure:"prometheus"`
	Tracing    TracingConfig    `mapstructure:"tracing"`
}

// TracingConfig 链路追踪配置（OpenTelemetry）
type TracingConfig struct {
	Enable         bool   `mapstructure:"enable"`
	ServiceName    string `mapstructure:"service_name"`
	ExportEndpoint string `mapstructure:"export_endpoint"`
	Insecure       bool   `mapstructure:"insecure"`
}

// PrometheusConfig Prometheus 配置
type PrometheusConfig struct {
	Enable bool `mapstructure:"enable"`
	Port   int  `mapstructure:"port"`
}

// LoadConfig 加载配置文件
func LoadConfig(configPath string) (*Config, error) {
	viper.SetConfigFile(configPath)
	viper.AutomaticEnv()
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	if err := viper.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("无法读取配置文件: %w", err)
	}

	var config Config
	if err := viper.Unmarshal(&config); err != nil {
		return nil, fmt.Errorf("无法解析配置文件: %w", err)
	}

	// 替换环境变量
	if err := replaceEnvVars(&config); err != nil {
		return nil, err
	}

	return &config, nil
}

// replaceEnvVars 替换配置中的环境变量
func replaceEnvVars(config *Config) error {
	// 替换模型 API Key
	for provider, providerConfig := range config.Model.LLM.Providers {
		if strings.HasPrefix(providerConfig.APIKey, "$") {
			envVar := strings.TrimPrefix(strings.TrimSuffix(providerConfig.APIKey, "}"), "${")
			if val := os.Getenv(envVar); val != "" {
				providerConfig.APIKey = val
				config.Model.LLM.Providers[provider] = providerConfig
			}
		}
	}

	for provider, providerConfig := range config.Model.Embedding.Providers {
		if strings.HasPrefix(providerConfig.APIKey, "$") {
			envVar := strings.TrimPrefix(strings.TrimSuffix(providerConfig.APIKey, "}"), "${")
			if val := os.Getenv(envVar); val != "" {
				providerConfig.APIKey = val
				config.Model.Embedding.Providers[provider] = providerConfig
			}
		}
	}

	for provider, providerConfig := range config.Model.Vision.Providers {
		if strings.HasPrefix(providerConfig.APIKey, "$") {
			envVar := strings.TrimPrefix(strings.TrimSuffix(providerConfig.APIKey, "}"), "${")
			if val := os.Getenv(envVar); val != "" {
				providerConfig.APIKey = val
				config.Model.Vision.Providers[provider] = providerConfig
			}
		}
	}

	return nil
}

// LoadAPIConfig 加载 API 配置（仅 configs/api.yaml）
func LoadAPIConfig() (*Config, error) {
	return LoadConfig("configs/api.yaml")
}

// LoadAPIConfigWithModel 加载 API 配置并合并 model 配置，便于 API 使用 LLM/Embedding；storage 仍来自 api.yaml（缺省为 memory）
func LoadAPIConfigWithModel() (*Config, error) {
	cfg, err := LoadConfig("configs/api.yaml")
	if err != nil {
		return nil, err
	}
	modelCfg, err := LoadConfig("configs/model.yaml")
	if err == nil {
		cfg.Model = modelCfg.Model
	}
	return cfg, nil
}

// LoadWorkerConfig 加载 Worker 配置（仅 configs/worker.yaml）
func LoadWorkerConfig() (*Config, error) {
	return LoadConfig("configs/worker.yaml")
}

// LoadWorkerConfigWithModel 加载 Worker 配置并合并 model 配置，便于 Worker 执行 Agent Job 时使用 LLM/Embedding。
// model 路径解析为与 worker 配置同目录（configs/），避免 cwd 导致 model.yaml 未加载。
func LoadWorkerConfigWithModel() (*Config, error) {
	cfg, err := LoadConfig("configs/worker.yaml")
	if err != nil {
		return nil, err
	}
	modelPath := "configs/model.yaml"
	if absWorker, errAbs := filepath.Abs("configs/worker.yaml"); errAbs == nil {
		modelPath = filepath.Join(filepath.Dir(absWorker), "model.yaml")
	}
	modelCfg, err := LoadConfig(modelPath)
	if err == nil {
		cfg.Model = modelCfg.Model
	} else {
		log.Printf("[config] 未加载 model 配置 %q，Worker 将无 LLM 配置: %v", modelPath, err)
	}
	return cfg, nil
}

// LoadModelConfig 加载模型配置
func LoadModelConfig() (*Config, error) {
	return LoadConfig("configs/model.yaml")
}
