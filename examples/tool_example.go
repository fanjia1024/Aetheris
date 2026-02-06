package main

import (
	"context"
	"fmt"
	"os"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino-ext/components/model/openai"
)

func main() {
	ctx := context.Background()

	// 创建 OpenAI ChatModel
	chatModel, err := openai.NewChatModel(ctx, &openai.ChatModelConfig{
		Model:  "gpt-4o",
		APIKey: os.Getenv("OPENAI_API_KEY"),
	})
	if err != nil {
		fmt.Printf("创建 ChatModel 失败: %v\n", err)
		return
	}

	// 创建工具
	calculatorTool := tool.NewBaseTool(
		"calculator",
		"执行数学计算",
		func(ctx context.Context, input string) (string, error) {
			// 这里可以实现实际的计算逻辑
			return fmt.Sprintf("计算结果: %s", input), nil
		},
	)

	// 创建 ChatModelAgent
	agent, err := adk.NewChatModelAgent(ctx, &adk.ChatModelAgentConfig{
		Model: chatModel,
		ToolsConfig: adk.ToolsConfig{
			ToolsNodeConfig: compose.ToolsNodeConfig{
				Tools: []tool.BaseTool{calculatorTool},
			},
		},
	})
	if err != nil {
		fmt.Printf("创建 Agent 失败: %v\n", err)
		return
	}

	// 创建 Runner
	runner := adk.NewRunner(ctx, adk.RunnerConfig{
		Agent: agent,
	})

	// 执行查询
	iter := runner.Query(ctx, "计算 1 + 1")

	// 处理结果
	for {
		event, ok := iter.Next()
		if !ok {
			break
		}
		fmt.Printf("事件类型: %s\n", event.Type)
		if event.Message != nil {
			fmt.Printf("内容: %s\n", event.Message.Content)
		}
	}
}
