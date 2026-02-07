package main

import (
	"context"
	"fmt"

	"github.com/cloudwego/eino/compose"
)

// Input 输入
type Input struct {
	Query string `json:"query"`
}

// Output 输出
type Output struct {
	Result string `json:"result"`
}

func main() {
	ctx := context.Background()

	// 创建图
	graph := compose.NewGraph[*Input, *Output]()

	// 添加验证节点
	graph.AddLambdaNode("validate", compose.InvokableLambda(func(ctx context.Context, input *Input) (*Output, error) {
		if input.Query == "" {
			return nil, fmt.Errorf("查询不能为空")
		}
		return &Output{Result: input.Query}, nil
	}))

	// 添加格式化节点（输入为上一节点 validate 的 *Output）
	graph.AddLambdaNode("format", compose.InvokableLambda(func(ctx context.Context, input *Output) (*Output, error) {
		return &Output{Result: fmt.Sprintf("格式化结果: %s", input.Result)}, nil
	}))

	// 添加边
	graph.AddEdge(compose.START, "validate")
	graph.AddEdge("validate", "format")
	graph.AddEdge("format", compose.END)

	// 编译图
	runnable, err := graph.Compile(ctx)
	if err != nil {
		fmt.Printf("编译图失败: %v\n", err)
		return
	}

	// 执行
	output, err := runnable.Invoke(ctx, &Input{Query: "Hello, workflow!"})
	if err != nil {
		fmt.Printf("执行失败: %v\n", err)
		return
	}

	fmt.Printf("执行结果: %s\n", output.Result)
}
