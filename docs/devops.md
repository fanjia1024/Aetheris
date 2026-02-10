# Eino Dev 开发套件说明

本文说明在本项目中使用 [Eino Dev](https://www.cloudwego.io/zh/docs/eino/core_modules/devops/) 应用开发工具链的方式，包括插件安装、调试入口与现有 pipeline 的关系。

## 概述

Eino Dev 是 CloudWeGo 提供的 **IDE 插件**，用于 Eino 编排的可视化开发与调试：

- **可视化编排**：通过拖拽组件构建 Graph、生成代码、导入导出（详见 [可视化编排插件功能指南](https://www.cloudwego.io/zh/docs/eino/core_modules/devops/visual_orchestration_plugin_guide/)）。
- **可视化调试**：渲染编排拓扑、从任意节点用 mock 数据发起 Test Run、查看每节点输入输出（详见 [可视化调试插件功能指南](https://www.cloudwego.io/zh/docs/eino/core_modules/devops/visual_debug_plugin_guide/)）。

插件通过连接进程内由 **eino-ext/devops** 启动的本地 HTTP 服务发现已 **Compile** 过的 Graph/Chain，因此需要先在本进程中调用 `devops.Init()`，再编译至少一个编排，并保持进程运行。

## 版本对应与 IDE 支持

| 插件版本 | GoLand | VS Code | eino-ext/devops 版本 |
| -------- | ------ | ------- | -------------------- |
| 1.1.0    | 2023.2+ | 1.97.x  | 0.1.0                |

本项目当前使用 **eino-ext/devops@v0.1.8**（与现有 eino 兼容；若需与插件 1.1.0 严格对应可尝试 0.1.0，需确认与当前 eino 版本兼容）。

- **GoLand**：Settings → Plugins → Marketplace 搜索 “Eino Dev” 并安装。
- **VS Code**：Extensions 搜索 “Eino Dev” 并安装。

## 本仓库使用步骤

### 1. 依赖

依赖已纳入 `go.mod`，无需额外操作。若需升级：

```bash
go get github.com/cloudwego/eino-ext/devops@latest
go mod tidy
```

### 2. 启动调试入口

本项目采用 **独立 dev 入口**（方案 A），不修改 api/worker：

```bash
go run ./cmd/devops
```

该命令会：

1. 调用 `devops.Init(ctx)` 启动 Eino Dev 调试服务（默认 `127.0.0.1:52538`）。
2. 注册并编译若干示例 Graph（如 validate→format、echo），供插件发现。
3. 阻塞直至收到 SIGINT/SIGTERM。

请保持该进程运行，以便 IDE 插件连接。

### 3. 在 IDE 中配置连接

1. 打开 Eino Dev 面板（GoLand 右侧 “Eino Dev” 图标；VS Code 底部 “Eino Dev”）。
2. 进入调试功能，点击 “Configure Address”（或类似入口）。
3. 输入 `127.0.0.1:52538`（默认端口），确认。
4. 连接成功后，在编排列表中选择已编译的 Graph（如包含 validate、format 或 echo 的图）。
5. 使用 **Test Run**：从 START 或从某一节点开始，按提示填写 mock 输入（如 `{"query":"hello"}`），查看各节点输入输出与执行顺序。

### 4. 自定义端口（可选）

若需修改调试服务端口，可在 `cmd/devops/main.go` 中为 `devops.Init` 增加选项（需改代码后重新 `go run ./cmd/devops`）：

```go
devops.Init(ctx, devops.WithDevServerPort("52600"))
```

并在 IDE 中配置对应地址（如 `127.0.0.1:52600`）。

### 5. 自定义类型与 interface 入参（可选）

当节点输入为 `interface{}` 或自定义类型时，可在 `Init` 时通过 `devops.AppendType` 注册具体类型，以便在插件 Test Run 的 mock 输入中选择该类型并填写 `_value`。详见官方 [可视化调试指南 - Specify Implementation Type](https://www.cloudwego.io/docs/eino/core_modules/devops/visual_debug_plugin_guide/)。

## 与现有 pipeline 的关系

本项目的 **query_pipeline** 与 **ingest_pipeline** 由 [WorkflowExecutor](internal/runtime/eino/workflow_executors.go) 实现（loader→parser→splitter→embedding→indexer 等），并非单一 `compose.Graph`，因此 **不会** 自动出现在 Eino Dev 插件的编排列表中。

- 若要用插件 **调试“图”编排**：请运行 `go run ./cmd/devops`，使用其中注册的示例 Graph，或参考 [examples/workflow](examples/workflow) 与 [internal/runtime/eino/workflow.go](internal/runtime/eino/workflow.go) 自行在 `cmd/devops` 中增加 Graph 并 `Compile`。
- 生产环境的检索与入库仍由 api/worker 中的 query_pipeline、ingest_pipeline 执行，与是否运行 `cmd/devops` 无关。

## 参考链接

- [Eino Dev: 应用开发工具链](https://www.cloudwego.io/zh/docs/eino/core_modules/devops/)
- [Eino Dev 插件安装指南](https://www.cloudwego.io/zh/docs/eino/core_modules/devops/ide_plugin_guide/)
- [Eino Dev 可视化编排插件功能指南](https://www.cloudwego.io/zh/docs/eino/core_modules/devops/visual_orchestration_plugin_guide/)
- [Eino Dev 可视化调试插件功能指南](https://www.cloudwego.io/zh/docs/eino/core_modules/devops/visual_debug_plugin_guide/)
- [eino-examples devops/debug](https://github.com/cloudwego/eino-examples)（官方调试示例）
