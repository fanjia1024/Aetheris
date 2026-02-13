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

.PHONY: build run stop clean test vet fmt fmt-check tidy help

# 默认目标：帮助
help:
	@echo "用法:"
	@echo "  make build   - 构建 api、worker、cli 到 $(BIN_DIR)/"
	@echo "  make run     - 构建并后台启动 API + Worker（一键启动所有服务）"
	@echo "  make stop    - 停止由 make run 启动的 API 与 Worker"
	@echo "  make clean   - 删除 $(BIN_DIR)/"
	@echo "  make test    - 运行测试"
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
	@mkdir -p $(BIN_DIR)
	@if [ -f $(API_PID) ]; then kill -0 $$(cat $(API_PID)) 2>/dev/null && { echo "API 已在运行 (PID $$(cat $(API_PID))), 先执行 make stop"; exit 1; }; fi
	@if [ -f $(WORKER_PID) ]; then kill -0 $$(cat $(WORKER_PID)) 2>/dev/null && { echo "Worker 已在运行 (PID $$(cat $(WORKER_PID))), 先执行 make stop"; exit 1; }; fi
	$(API_BIN) > $(API_LOG) 2>&1 & echo $$! > $(API_PID)
	$(WORKER_BIN) > $(WORKER_LOG) 2>&1 & echo $$! > $(WORKER_PID)
	@echo "API 已启动 (PID $$(cat $(API_PID))), 日志: $(API_LOG)"
	@echo "Worker 已启动 (PID $$(cat $(WORKER_PID))), 日志: $(WORKER_LOG)"
	@echo "停止服务: make stop"
	@echo "健康检查: curl http://localhost:8080/api/health"

# 停止由 make run 启动的进程
stop:
	@[ -f $(API_PID) ] && kill $$(cat $(API_PID)) 2>/dev/null && echo "已停止 API ($$(cat $(API_PID)))" || true
	@[ -f $(WORKER_PID) ] && kill $$(cat $(WORKER_PID)) 2>/dev/null && echo "已停止 Worker ($$(cat $(WORKER_PID)))" || true
	@rm -f $(API_PID) $(WORKER_PID)

clean:
	rm -rf $(BIN_DIR)

test:
	go test -v -race -count=1 ./...

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
