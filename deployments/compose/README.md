# Compose 部署（API + 2 Worker + Postgres）

用于 v1.0 可部署性验收：单机 docker-compose 跑通、两 Worker 一 API 正常运行、重启恢复。

## 启动

从**仓库根目录**执行：

```bash
docker compose -f deployments/compose/docker-compose.yml up -d --build
```

或使用一键脚本（推荐）：

```bash
./scripts/local-2.0-stack.sh start
```

或进入本目录（需能访问上级的 `internal/` 与 `configs/`）：

```bash
cd deployments/compose && docker compose up -d --build
```

### 本地 Ollama（推荐用于发布测试）

Compose 默认走 OpenAI 兼容端点并指向本机 Ollama：
- base URL: `http://host.docker.internal:11434/v1`
- 默认模型: `llama3:latest`（可通过 `OLLAMA_MODEL` 覆盖）

启动前请先在本机准备 Ollama 模型：

```bash
ollama serve
ollama pull llama3:latest
```

若使用其他模型：

```bash
OLLAMA_MODEL=llama3.1:8b docker compose -f deployments/compose/docker-compose.yml up -d --build
```

## 服务

| 服务     | 端口  | 说明 |
|----------|-------|------|
| postgres | 5432  | 事件流、job 元数据、agent 状态 |
| api      | 8080  | HTTP API（jobstore=postgres 时不启 Scheduler） |
| worker1  | -     | Agent Job Claim 执行，WORKER_ID=worker-1 |
| worker2  | -     | Agent Job Claim 执行，WORKER_ID=worker-2 |

## 验收

1. 健康检查：`curl http://localhost:8080/api/health`
2. 登录获取 JWT：`curl -X POST http://localhost:8080/api/login -H "Content-Type: application/json" -d '{"username":"admin","password":"admin"}'`
3. 创建 Agent：`curl -X POST http://localhost:8080/api/agents/ -H "Authorization: Bearer <token>" -H "Content-Type: application/json" -d '{"name":"test"}'`
4. 发消息（创建 Job）：`curl -X POST http://localhost:8080/api/agents/<agent_id>/message -H "Authorization: Bearer <token>" -H "Content-Type: application/json" -d '{"message":"hello"}'`
5. 重启 API 或 Worker 后，Job 由 Postgres 持久化，可继续执行或由新 Worker 接管。

## 管理命令（脚本）

```bash
./scripts/local-2.0-stack.sh status   # 查看服务状态
./scripts/local-2.0-stack.sh logs     # 查看实时日志
./scripts/local-2.0-stack.sh health   # 健康检查
./scripts/local-2.0-stack.sh stop     # 停止并清理栈
```

## 升级已有库

若 Postgres 已存在且无 `cancel_requested_at` 列，执行：

```sql
ALTER TABLE jobs ADD COLUMN IF NOT EXISTS cancel_requested_at TIMESTAMPTZ;
```
