# Aetheris / CoRag — 一键构建并启动所有服务
# 用法: make build  构建; make run  构建并启动 API + Worker; make stop  停止

BIN_DIR   := bin
API_BIN   := $(BIN_DIR)/api
WORKER_BIN := $(BIN_DIR)/worker
CLI_BIN   := $(BIN_DIR)/aetheris
API_PID   := $(BIN_DIR)/api.pid
WORKER_PID := $(BIN_DIR)/worker.pid
API_LOG   := $(BIN_DIR)/api.log
WORKER_LOG := $(BIN_DIR)/worker.log

.PHONY: build run run-all run-api run-worker stop clean test test-integration vet fmt fmt-check tidy docker-build docker-run docker-stop release-2.0 help

# 默认目标：帮助
help:
	@echo "用法:"
	@echo "  make build   - 构建 api、worker、cli 到 $(BIN_DIR)/"
	@echo "  make run     - 构建并后台启动 API + Worker（一键启动所有服务）"
	@echo "  make run-api - 仅后台启动 API（会先 build）"
	@echo "  make run-worker - 仅后台启动 Worker（会先 build）"
	@echo "  make run-all - 等价 make run"
	@echo "  make stop    - 停止由 make run 启动的 API 与 Worker"
	@echo "  make clean   - 删除 $(BIN_DIR)/"
	@echo "  make test    - 运行测试"
	@echo "  make test-integration - 运行关键集成测试（runtime + http）"
	@echo "  make docker-build - 构建 API/Worker 容器镜像（deployments/compose/Dockerfile）"
	@echo "  make docker-run   - 使用 compose 启动本地 2.0 栈"
	@echo "  make docker-stop  - 使用 compose 停止本地 2.0 栈"
	@echo "  make release-2.0  - 执行 2.0 发布前检查脚本"
	@echo "  make vet     - go vet"
	@echo "  make fmt       - gofmt -w"
	@echo "  make fmt-check - 检查格式（未通过则 exit 1，与 CI 一致）"
	@echo "  make tidy    - go mod tidy"

# 构建所有二进制
build:
	@mkdir -p $(BIN_DIR)
	go build -o $(API_BIN) ./cmd/api
	go build -o $(WORKER_BIN) ./cmd/worker
	go build -o $(CLI_BIN) ./cmd/cli
	@echo "已构建: $(API_BIN) $(WORKER_BIN) $(CLI_BIN)"

# 构建并启动 API + Worker（后台），并写入 PID 与日志
run: build
	@$(MAKE) run-api
	@$(MAKE) run-worker
	@echo "停止服务: make stop"
	@echo "健康检查: curl http://localhost:8080/api/health"

run-all: run

run-api: build
	@mkdir -p $(BIN_DIR)
	@if [ -f $(API_PID) ]; then kill -0 $$(cat $(API_PID)) 2>/dev/null && { echo "API 已在运行 (PID $$(cat $(API_PID))), 先执行 make stop"; exit 1; }; fi
	$(API_BIN) > $(API_LOG) 2>&1 & echo $$! > $(API_PID)
	@echo "API 已启动 (PID $$(cat $(API_PID))), 日志: $(API_LOG)"

run-worker: build
	@mkdir -p $(BIN_DIR)
	@if [ -f $(WORKER_PID) ]; then kill -0 $$(cat $(WORKER_PID)) 2>/dev/null && { echo "Worker 已在运行 (PID $$(cat $(WORKER_PID))), 先执行 make stop"; exit 1; }; fi
	$(WORKER_BIN) > $(WORKER_LOG) 2>&1 & echo $$! > $(WORKER_PID)
	@echo "Worker 已启动 (PID $$(cat $(WORKER_PID))), 日志: $(WORKER_LOG)"

# 停止由 make run 启动的进程
stop:
	@[ -f $(API_PID) ] && kill $$(cat $(API_PID)) 2>/dev/null && echo "已停止 API ($$(cat $(API_PID)))" || true
	@[ -f $(WORKER_PID) ] && kill $$(cat $(WORKER_PID)) 2>/dev/null && echo "已停止 Worker ($$(cat $(WORKER_PID)))" || true
	@rm -f $(API_PID) $(WORKER_PID)

clean:
	rm -rf $(BIN_DIR)

test:
	go test -v -race -count=1 ./...

test-integration:
	go test -v ./internal/agent/runtime/executor ./internal/api/http

docker-build:
	docker build -f deployments/compose/Dockerfile -t aetheris/runtime:local .

docker-run:
	./scripts/local-2.0-stack.sh start

docker-stop:
	./scripts/local-2.0-stack.sh stop

release-2.0:
	./scripts/release-2.0.sh

vet:
	go vet ./...

fmt:
	gofmt -w .

fmt-check:
	@if [ -n "$$(gofmt -l .)" ]; then \
		echo "The following files are not formatted with gofmt:"; \
		gofmt -l .; \
		exit 1; \
	fi

tidy:
	go mod tidy
