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

package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"rag-platform/internal/app/worker"
	"rag-platform/pkg/config"
)

func main() {
	// 加载配置（合并 configs/model.yaml，需 LLM 时请从项目根启动并确保 configs/model.yaml 存在；修改代码后需重新编译并重启 Worker）
	cfg, err := config.LoadWorkerConfigWithModel()
	if err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}

	// 初始化应用
	app, err := worker.NewApp(cfg)
	if err != nil {
		log.Fatalf("初始化应用失败: %v", err)
	}

	// 启动应用
	if err := app.Start(); err != nil {
		log.Fatalf("启动应用失败: %v", err)
	}

	// 等待中断信号
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	// 优雅关闭
	ctx, cancel := context.WithTimeout(context.Background(), 30)
	defer cancel()

	if err := app.Shutdown(ctx); err != nil {
		log.Printf("关闭应用失败: %v", err)
	}

	fmt.Println("应用已关闭")
}
