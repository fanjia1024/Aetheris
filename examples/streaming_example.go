package main

import (
	"context"
	"fmt"
	"os"

	"github.com/cloudwego/eino/adk"
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

	// 创建 ChatModelAgent
	agent, err := adk.NewChatModelAgent(ctx, &adk.ChatModelAgentConfig{
		Model: chatModel,
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
	iter := runner.Query(ctx, "Hello, who are you? Please tell me about yourself.")

	// 处理流式结果
	fmt.Println("开始处理流式结果:")
	for {
		event, ok := iter.Next()
		if !ok {
			break
		}
		
		switch event.Type {
		case adk.EventTypeMessage:
			if event.Message != nil {
				fmt.Printf("消息: %s\n", event.Message.Content)
			}
		case adk.EventTypeToolCall:
			if event.ToolCall != nil {
				fmt.Printf("工具调用: %s\n", event.ToolCall.Name)
			}
		case adk.EventTypeToolResponse:
			if event.ToolResponse != nil {
				fmt.Printf("工具响应: %s\n", event.ToolResponse.Result)
			}
		case adk.EventTypeError:
			if event.Error != nil {
				fmt.Printf("错误: %s\n", event.Error.Message)
			}
		default:
			fmt.Printf("未知事件类型: %s\n", event.Type)
		}
	}

	fmt.Println("流式处理完成")
}
