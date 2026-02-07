package eino

import (
	"context"
	"encoding/json"
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

// retrieverInput 检索工具入参（JSON）
type retrieverInput struct {
	Query      string `json:"query"`
	Collection string `json:"collection"`
	TopK       int    `json:"top_k"`
}

// CreateRetrieverTool 创建检索工具（若 engine.Retriever 已注入则对接真实检索）
func CreateRetrieverTool(engine *Engine) tool.BaseTool {
	if engine != nil && engine.Retriever != nil {
		t, err := utils.InferTool("retriever", "检索相关文档。input 为 JSON：{\"query\":\"问题\",\"collection\":\"集合名\",\"top_k\":10}", func(ctx context.Context, input string) (string, error) {
			var in retrieverInput
			if err := json.Unmarshal([]byte(input), &in); err != nil {
				in = retrieverInput{Query: input, Collection: "default", TopK: 10}
			}
			if in.Collection == "" {
				in.Collection = "default"
			}
			if in.TopK <= 0 {
				in.TopK = 10
			}
			chunks, err := engine.Retriever.Retrieve(ctx, in.Query, in.Collection, in.TopK)
			if err != nil {
				return "", err
			}
			out, _ := json.Marshal(chunks)
			return string(out), nil
		})
		if err != nil {
			panic(err)
		}
		return t
	}
	return createPlaceholderTool("retriever", "检索相关文档")
}

// CreateGeneratorTool 创建生成工具（若 engine.Generator 已注入则对接真实生成）
func CreateGeneratorTool(engine *Engine) tool.BaseTool {
	if engine != nil && engine.Generator != nil {
		t, err := utils.InferTool("generator", "根据提示生成回答。input 为提示文本或 JSON {\"prompt\":\"...\"}", func(ctx context.Context, input string) (string, error) {
			prompt := input
			var in struct {
				Prompt string `json:"prompt"`
			}
			if err := json.Unmarshal([]byte(input), &in); err == nil && in.Prompt != "" {
				prompt = in.Prompt
			}
			return engine.Generator.Generate(ctx, prompt)
		})
		if err != nil {
			panic(err)
		}
		return t
	}
	return createPlaceholderTool("generator", "生成回答")
}

// docInput 文档类工具入参（path 或 文档 JSON）
type docInput struct {
	Path string `json:"path"`
}

// CreateDocumentLoaderTool 创建文档加载工具
func CreateDocumentLoaderTool(engine *Engine) tool.BaseTool {
	if engine != nil && engine.DocumentLoader != nil {
		t, err := utils.InferTool("document_loader", "加载文档。input 为 JSON {\"path\":\"文件路径\"} 或直接路径", func(ctx context.Context, input string) (string, error) {
			var in docInput
			if err := json.Unmarshal([]byte(input), &in); err != nil {
				in = docInput{Path: input}
			}
			arg := in.Path
			if arg == "" {
				arg = input
			}
			result, err := engine.DocumentLoader.Load(ctx, arg)
			if err != nil {
				return "", err
			}
			out, _ := json.Marshal(result)
			return string(out), nil
		})
		if err != nil {
			panic(err)
		}
		return t
	}
	return createPlaceholderTool("document_loader", "加载文档")
}

// CreateDocumentParserTool 创建文档解析工具
func CreateDocumentParserTool(engine *Engine) tool.BaseTool {
	if engine != nil && engine.DocumentParser != nil {
		t, err := utils.InferTool("document_parser", "解析文档。input 为文档对象 JSON", func(ctx context.Context, input string) (string, error) {
			var doc interface{}
			_ = json.Unmarshal([]byte(input), &doc)
			result, err := engine.DocumentParser.Parse(ctx, doc)
			if err != nil {
				return "", err
			}
			out, _ := json.Marshal(result)
			return string(out), nil
		})
		if err != nil {
			panic(err)
		}
		return t
	}
	return createPlaceholderTool("document_parser", "解析文档")
}

// CreateSplitterTool 创建文档切片工具
func CreateSplitterTool(engine *Engine) tool.BaseTool {
	if engine != nil && engine.DocumentSplitter != nil {
		t, err := utils.InferTool("splitter", "文档切片。input 为文档对象 JSON", func(ctx context.Context, input string) (string, error) {
			var doc interface{}
			_ = json.Unmarshal([]byte(input), &doc)
			result, err := engine.DocumentSplitter.Split(ctx, doc)
			if err != nil {
				return "", err
			}
			out, _ := json.Marshal(result)
			return string(out), nil
		})
		if err != nil {
			panic(err)
		}
		return t
	}
	return createPlaceholderTool("splitter", "文档切片")
}

// CreateEmbeddingTool 创建文本向量化工具
func CreateEmbeddingTool(engine *Engine) tool.BaseTool {
	if engine != nil && engine.DocumentEmbedding != nil {
		t, err := utils.InferTool("embedding", "文本向量化。input 为文档对象 JSON", func(ctx context.Context, input string) (string, error) {
			var doc interface{}
			_ = json.Unmarshal([]byte(input), &doc)
			result, err := engine.DocumentEmbedding.Embed(ctx, doc)
			if err != nil {
				return "", err
			}
			out, _ := json.Marshal(result)
			return string(out), nil
		})
		if err != nil {
			panic(err)
		}
		return t
	}
	return createPlaceholderTool("embedding", "文本向量化")
}

// CreateIndexBuilderTool 创建索引构建工具
func CreateIndexBuilderTool(engine *Engine) tool.BaseTool {
	if engine != nil && engine.DocumentIndexer != nil {
		t, err := utils.InferTool("index_builder", "构建索引。input 为文档对象 JSON", func(ctx context.Context, input string) (string, error) {
			var doc interface{}
			_ = json.Unmarshal([]byte(input), &doc)
			result, err := engine.DocumentIndexer.Index(ctx, doc)
			if err != nil {
				return "", err
			}
			out, _ := json.Marshal(result)
			return string(out), nil
		})
		if err != nil {
			panic(err)
		}
		return t
	}
	return createPlaceholderTool("index_builder", "构建索引")
}

// GetDefaultTools 获取默认工具列表（需要传入 engine 以支持注入组件）
func GetDefaultTools(engine *Engine) []tool.BaseTool {
	return []tool.BaseTool{
		CreateRetrieverTool(engine),
		CreateGeneratorTool(engine),
		CreateDocumentLoaderTool(engine),
		CreateDocumentParserTool(engine),
		CreateSplitterTool(engine),
		CreateEmbeddingTool(engine),
		CreateIndexBuilderTool(engine),
	}
}
