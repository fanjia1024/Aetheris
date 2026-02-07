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
	iter := runner.Query(ctx, "Hello, who are you?")

	// 处理结果
	for {
		event, ok := iter.Next()
		if !ok {
			break
		}
		if event.Err != nil {
			fmt.Printf("错误: %v\n", event.Err)
			continue
		}
		if event.Output != nil && event.Output.MessageOutput != nil {
			msg := event.Output.MessageOutput.Message
			if msg != nil && msg.Content != "" {
				fmt.Printf("消息: %s\n", msg.Content)
			}
		}
	}
}
