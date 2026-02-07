# 端到端测试

本文档描述「上传 → 解析 → 切分 → 索引 → 检索」的完整测试步骤，支持使用 **PDF** 或 **AGENTS.md** 作为测试文件。

## 前置条件

- **Go**：已安装 Go（建议 1.21+）。
- **配置**：`configs/model.yaml` 已配置；若使用 OpenAI，需设置环境变量 `OPENAI_API_KEY`。未配置时 API 仍可启动，但查询/上传可能走占位或返回错误。
- **存储**：当前默认为 **memory** 存储，进程重启后数据清空；仅用于本地验证。

## 1. 启动 API

```bash
go run ./cmd/api
```

默认监听 `http://localhost:8080`。确认健康检查：

```bash
curl http://localhost:8080/api/health
```

预期：返回 200，表示服务正常。

## 2. 上传文档

### 使用 PDF

准备一个 PDF 文件，执行：

```bash
curl -X POST http://localhost:8080/api/documents/upload \
  -F "file=@/path/to/your.pdf"
```

**预期**：返回 200，响应中包含 `doc_id`、`chunks` 等；PDF 正文会在 Loader 阶段被提取为文本，再经解析、切分、向量化并写入索引。

### 使用 AGENTS.md（无 PDF 时快速验证）

可将项目根目录的 `AGENTS.md` 作为测试文件（或复制为 `AGENTS.txt`）走同一套流程：

```bash
curl -X POST http://localhost:8080/api/documents/upload \
  -F "file=@./AGENTS.md"
```

或：

```bash
cp AGENTS.md AGENTS.txt
curl -X POST http://localhost:8080/api/documents/upload \
  -F "file=@./AGENTS.txt"
```

**预期**：返回 200，与 PDF 上传流程一致。

## 3. 查看文档列表

```bash
curl http://localhost:8080/api/documents/
```

预期：返回 200，列表中包含刚上传的文档（含 `id`、元数据等）。

## 4. 发起查询

使用与已上传文档内容相关的问题进行检索与生成：

```bash
curl -X POST http://localhost:8080/api/query \
  -H "Content-Type: application/json" \
  -d '{"query": "你的问题", "top_k": 10}'
```

**预期**：返回 200，响应中包含基于检索的 `answer`（若配置了 LLM 与 Embedding）；若仅使用 memory 占位，可能返回占位或错误，属预期。

## 5. 验收要点

- **PDF**：上传真实 PDF 后，文档正文为可读文本（非乱码）；切分、向量化、索引正常；用与该 PDF 内容相关的问题调用 `/api/query` 能返回基于检索的答案。
- **AGENTS.md / 文本文件**：上传后同样能完成上传、列表、查询整条链路。
- 默认 **memory** 存储下，重启 API 后数据会清空，需重新上传后再查询。

## 可选：一键测试脚本

项目提供脚本 `scripts/test-e2e.sh`，在 API 已启动的前提下，可传入待上传文件路径，自动执行：健康检查 → 上传 → 列表 → 查询。用法示例：

```bash
./scripts/test-e2e.sh ./AGENTS.md
# 或
./scripts/test-e2e.sh /path/to/your.pdf
```

脚本会输出关键响应字段，便于人工核对。
