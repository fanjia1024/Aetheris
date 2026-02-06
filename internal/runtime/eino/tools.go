package eino

import (
	"context"
	"fmt"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"
)

// ToolConfig 工具配置
type ToolConfig struct {
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Parameters  map[string]string `json:"parameters"`
}

func createPlaceholderTool(name, desc string) tool.BaseTool {
	t, err := utils.InferTool(name, desc, func(ctx context.Context, input string) (string, error) {
		return fmt.Sprintf("%s 结果: %s", name, input), nil
	})
	if err != nil {
		panic(err)
	}
	return t
}

// CreateRetrieverTool 创建检索工具（占位）
func CreateRetrieverTool() tool.BaseTool {
	return createPlaceholderTool("retriever", "检索相关文档")
}

// CreateGeneratorTool 创建生成工具（占位）
func CreateGeneratorTool() tool.BaseTool {
	return createPlaceholderTool("generator", "生成回答")
}

// CreateDocumentLoaderTool 创建文档加载工具（占位）
func CreateDocumentLoaderTool() tool.BaseTool {
	return createPlaceholderTool("document_loader", "加载文档")
}

// CreateDocumentParserTool 创建文档解析工具（占位）
func CreateDocumentParserTool() tool.BaseTool {
	return createPlaceholderTool("document_parser", "解析文档")
}

// CreateSplitterTool 创建文档切片工具（占位）
func CreateSplitterTool() tool.BaseTool {
	return createPlaceholderTool("splitter", "文档切片")
}

// CreateEmbeddingTool 创建文本向量化工具（占位）
func CreateEmbeddingTool() tool.BaseTool {
	return createPlaceholderTool("embedding", "文本向量化")
}

// CreateIndexBuilderTool 创建索引构建工具（占位）
func CreateIndexBuilderTool() tool.BaseTool {
	return createPlaceholderTool("index_builder", "构建索引")
}

// GetDefaultTools 获取默认工具列表
func GetDefaultTools() []tool.BaseTool {
	return []tool.BaseTool{
		CreateRetrieverTool(),
		CreateGeneratorTool(),
		CreateDocumentLoaderTool(),
		CreateDocumentParserTool(),
		CreateSplitterTool(),
		CreateEmbeddingTool(),
		CreateIndexBuilderTool(),
	}
}
