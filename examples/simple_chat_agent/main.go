// 最小 Aetheris Agent 示例：不启动 HTTP，直接 Run 一次对话。
// This demonstrates how an agent executes in Aetheris runtime with checkpointing.
// 运行：OPENAI_API_KEY=sk-xxx go run ./examples/simple_chat_agent
package main

import (
	"context"
	"fmt"
	"os"

	"rag-platform/pkg/agent"
)

func main() {
	ctx := context.Background()
	ag := agent.NewAgent()
	res, err := ag.Run(ctx, "Hello, who are you?")
	if err != nil {
		fmt.Fprintf(os.Stderr, "run error: %v\n", err)
		os.Exit(1)
	}
	fmt.Println(res.Answer)
	fmt.Printf("(steps=%d, duration=%s)\n", res.Steps, res.Duration)
}
