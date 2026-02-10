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

// devops 启动 Eino Dev 调试服务并注册示例 Graph，供 IDE 插件（Eino Dev）连接后进行可视化调试。
// 使用：go run ./cmd/devops；在 IDE 中配置连接地址 127.0.0.1:52538 后选择编排进行 Test Run。
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/cloudwego/eino-ext/devops"
	"github.com/cloudwego/eino/compose"
)

// DevInput 示例图输入（与 examples/workflow 一致便于插件调试）
type DevInput struct {
	Query string `json:"query"`
}

// DevOutput 示例图输出
type DevOutput struct {
	Result string `json:"result"`
}

// registerSimpleGraph 注册并编译一个简单 Graph，使 Eino Dev 插件能发现并调试
func registerSimpleGraph(ctx context.Context) error {
	g := compose.NewGraph[*DevInput, *DevOutput]()

	g.AddLambdaNode("validate", compose.InvokableLambda(func(ctx context.Context, input *DevInput) (*DevOutput, error) {
		if input == nil || input.Query == "" {
			return nil, fmt.Errorf("查询不能为空")
		}
		return &DevOutput{Result: input.Query}, nil
	}))

	g.AddLambdaNode("format", compose.InvokableLambda(func(ctx context.Context, input *DevOutput) (*DevOutput, error) {
		if input == nil {
			return &DevOutput{Result: ""}, nil
		}
		return &DevOutput{Result: fmt.Sprintf("格式化结果: %s", input.Result)}, nil
	}))

	g.AddEdge(compose.START, "validate")
	g.AddEdge("validate", "format")
	g.AddEdge("format", compose.END)

	_, err := g.Compile(ctx)
	if err != nil {
		return fmt.Errorf("compile simple graph: %w", err)
	}
	return nil
}

// registerEchoGraph 再注册一个极简 echo 图，便于插件中看到多个 artifact
func registerEchoGraph(ctx context.Context) error {
	g := compose.NewGraph[*DevInput, *DevOutput]()

	g.AddLambdaNode("echo", compose.InvokableLambda(func(ctx context.Context, input *DevInput) (*DevOutput, error) {
		if input == nil {
			return &DevOutput{Result: ""}, nil
		}
		return &DevOutput{Result: "echo: " + input.Query}, nil
	}))

	g.AddEdge(compose.START, "echo")
	g.AddEdge("echo", compose.END)

	_, err := g.Compile(ctx)
	if err != nil {
		return fmt.Errorf("compile echo graph: %w", err)
	}
	return nil
}

func main() {
	ctx := context.Background()

	// 1. 先初始化 Eino Dev 调试服务（必须在任何 Compile 之前调用）
	if err := devops.Init(ctx); err != nil {
		log.Fatalf("[eino dev] init failed: %v", err)
	}

	// 2. 注册并编译示例图，插件会通过已编译的 artifact 列表展示
	if err := registerSimpleGraph(ctx); err != nil {
		log.Fatalf("[eino dev] register simple graph: %v", err)
	}
	if err := registerEchoGraph(ctx); err != nil {
		log.Fatalf("[eino dev] register echo graph: %v", err)
	}

	log.Println("[eino dev] server listening on 127.0.0.1:52538; open Eino Dev in IDE and configure this address to debug")
	log.Println("[eino dev] press Ctrl+C to exit")

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	<-sigs

	log.Println("[eino dev] shutting down")
}
