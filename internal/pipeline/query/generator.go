package query

import (
	"fmt"
	"strings"
	"time"

	"rag-platform/internal/pipeline/common"
	"rag-platform/internal/model/llm"
)

// Generator 生成器
type Generator struct {
	name           string
	llmClient      *llm.Client
	maxContextSize int
	temperature    float64
}

// NewGenerator 创建新的生成器
func NewGenerator(llmClient *llm.Client, maxContextSize int, temperature float64) *Generator {
	if maxContextSize <= 0 {
		maxContextSize = 4096
	}
	if temperature <= 0 {
		temperature = 0.1
	}

	return &Generator{
		name:           "generator",
		llmClient:      llmClient,
		maxContextSize: maxContextSize,
		temperature:    temperature,
	}
}

// Name 返回组件名称
func (g *Generator) Name() string {
	return g.name
}

// Execute 执行生成操作
func (g *Generator) Execute(ctx *common.PipelineContext, input interface{}) (interface{}, error) {
	// 验证输入
	if err := g.Validate(input); err != nil {
		return nil, common.NewPipelineError(g.name, "输入验证失败", err)
	}

	// 生成回答
	inputData := input.(map[string]interface{})
	query, ok := inputData["query"].(*common.Query)
	if !ok {
		return nil, common.NewPipelineError(g.name, "输入类型错误", fmt.Errorf("expected query to be *common.Query"))
	}

	result, ok := inputData["retrieval_result"].(*common.RetrievalResult)
	if !ok {
		return nil, common.NewPipelineError(g.name, "输入类型错误", fmt.Errorf("expected retrieval_result to be *common.RetrievalResult"))
	}

	// 处理生成
	generationResult, err := g.generate(query, result)
	if err != nil {
		return nil, common.NewPipelineError(g.name, "生成回答失败", err)
	}

	return generationResult, nil
}

// Validate 验证输入
func (g *Generator) Validate(input interface{}) error {
	if input == nil {
		return common.ErrInvalidInput
	}

	inputData, ok := input.(map[string]interface{})
	if !ok {
		return fmt.Errorf("不支持的输入类型: %T", input)
	}

	if _, ok := inputData["query"].(*common.Query); !ok {
		return fmt.Errorf("query 不是 *common.Query 类型")
	}

	if _, ok := inputData["retrieval_result"].(*common.RetrievalResult); !ok {
		return fmt.Errorf("retrieval_result 不是 *common.RetrievalResult 类型")
	}

	if g.llmClient == nil {
		return fmt.Errorf("未初始化 LLM 客户端")
	}

	return nil
}

// ProcessQuery 处理查询
func (g *Generator) ProcessQuery(query *common.Query) (*common.Query, error) {
	// 这里可以添加查询预处理逻辑
	return query, nil
}

// generate 生成回答
func (g *Generator) generate(query *common.Query, result *common.RetrievalResult) (*common.GenerationResult, error) {
	startTime := time.Now()

	// 构建提示词
	prompt := g.buildPrompt(query, result)

	// 调用 LLM 生成回答
	response, err := g.llmClient.Generate(prompt, llm.GenerateOptions{
		Temperature:    g.temperature,
		MaxTokens:      1024,
		TopP:           0.9,
		FrequencyPenalty: 0.0,
		PresencePenalty:  0.0,
	})
	if err != nil {
		return nil, fmt.Errorf("调用 LLM 失败: %w", err)
	}

	// 提取引用
	references := g.extractReferences(result)

	// 创建生成结果
	generationResult := &common.GenerationResult{
		Answer:      response,
		References:  references,
		ProcessTime: time.Since(startTime),
	}

	return generationResult, nil
}

// buildPrompt 构建提示词
func (g *Generator) buildPrompt(query *common.Query, result *common.RetrievalResult) string {
	var prompt strings.Builder

	// 系统提示
	prompt.WriteString("你是一个专业的问答助手，基于提供的参考资料回答用户问题。\n")
	prompt.WriteString("请严格按照以下要求：\n")
	prompt.WriteString("1. 仅基于提供的参考资料回答问题\n")
	prompt.WriteString("2. 回答要准确、简洁、专业\n")
	prompt.WriteString("3. 不要添加参考资料中没有的信息\n")
	prompt.WriteString("4. 如果参考资料不足以回答问题，请明确说明\n")
	prompt.WriteString("\n")

	// 参考资料
	prompt.WriteString("参考资料：\n")
	for i, chunk := range result.Chunks {
		prompt.WriteString(fmt.Sprintf("[%d] %s\n", i+1, chunk.Content))
		prompt.WriteString("\n")
	}

	// 用户问题
	prompt.WriteString("用户问题：\n")
	prompt.WriteString(query.Text)
	prompt.WriteString("\n\n")

	// 回答格式
	prompt.WriteString("回答：")

	return prompt.String()
}

// extractReferences 提取引用
func (g *Generator) extractReferences(result *common.RetrievalResult) []string {
	var references []string

	for i, chunk := range result.Chunks {
		reference := fmt.Sprintf("[%d] 文档: %s, 切片: %d", i+1, chunk.DocumentID, chunk.Index)
		references = append(references, reference)
	}

	return references
}

// SetLLMClient 设置 LLM 客户端
func (g *Generator) SetLLMClient(client *llm.Client) {
	g.llmClient = client
}

// GetLLMClient 获取 LLM 客户端
func (g *Generator) GetLLMClient() *llm.Client {
	return g.llmClient
}

// SetMaxContextSize 设置最大上下文大小
func (g *Generator) SetMaxContextSize(size int) {
	if size > 0 {
		g.maxContextSize = size
	}
}

// SetTemperature 设置温度参数
func (g *Generator) SetTemperature(temperature float64) {
	if temperature > 0 {
		g.temperature = temperature
	}
}
