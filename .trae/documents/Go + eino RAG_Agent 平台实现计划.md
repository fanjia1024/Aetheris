# Go + eino RAG/Agent 平台实现计划

## 项目初始化与结构搭建

### 1. 项目基础结构
- 创建标准 Go 项目结构（cmd/internal/pkg）
- 配置 go.mod 和依赖管理
- 搭建配置管理系统

### 2. 核心服务框架
- **api-service**: 构建 HTTP/gRPC 对外接口
- **agent-service**: 集成 eino Runtime 作为核心编排引擎
- **index-service**: 实现离线索引与文档处理

## 核心模块实现

### 3. eino Runtime（系统大脑）
- 集成 eino 引擎
- 实现 Workflow/DAG 定义与执行
- 构建 Agent 调度系统
- 实现 Context/State 管理

### 4. Pipeline 系统
- **Ingest Pipeline**: 文档加载 → 解析 → 切片 → Embedding → 索引
- **Query Pipeline**: 检索 → 重排 → 生成 → 响应
- **Specialized Pipeline**: JSONL、HIVE、长文本处理

### 5. Splitter Engine
- 结构切片（文档/段落）
- 语义切片
- Token 切片

### 6. Model Abstraction Layer
- LLM 接口（支持多厂商）
- Embedding 接口
- Vision 接口（可选）
- 运行时模型切换

### 7. Storage Layer
- Metadata Store（MySQL/TiDB）
- Vector Store（Milvus/Vearch/ES）
- Object Store（S3/OSS）
- Cache（Redis/Local）

## API 与集成

### 8. API 层
- HTTP REST API
- gRPC 内部服务
- 中间件（权限/限流/监控）

### 9. 配置与部署
- 多环境配置
- Docker 容器化
- 部署脚本

## 实现原则

- **Single Orchestrator**: 所有 Pipeline 只能被 eino 调度
- **Go First**: 核心逻辑全部 Go 原生实现
- **Agent-Native**: 为 2025-2026 Agent 架构演进做准备
- **严格分层**: 禁止反向依赖，保持架构清晰

## 技术栈

- **语言**: Go 1.20+
- **核心引擎**: eino
- **API**: HTTP/REST + gRPC
- **存储**: MySQL/TiDB + Milvus/ES + S3/OSS + Redis
- **监控**: 集成可观测性工具

## 预期成果

- 完整的 Go + eino RAG/Agent 平台
- 支持复杂 Workflow/DAG 编排
- 高性能离线索引与在线检索
- 可扩展的 Agent 系统
- 生产级部署架构