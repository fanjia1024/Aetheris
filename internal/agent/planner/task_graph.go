package planner

import (
	"encoding/json"
	"time"
)

// TaskNodeType 节点类型
const (
	NodeTool     = "tool"
	NodeWorkflow = "workflow"
	NodeLLM      = "llm"
)

// TaskNode 任务图中的节点
type TaskNode struct {
	ID       string         `json:"id"`
	Type     string         `json:"type"` // tool / workflow / llm
	Config   map[string]any `json:"config,omitempty"`
	ToolName string         `json:"tool_name,omitempty"` // Type=tool 时使用
	Workflow string         `json:"workflow,omitempty"`  // Type=workflow 时使用
}

// TaskEdge 任务图中的边
type TaskEdge struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// TaskGraph 任务图：可序列化供 Checkpoint 保存
type TaskGraph struct {
	Nodes []TaskNode `json:"nodes"`
	Edges []TaskEdge `json:"edges"`
}

// Marshal 序列化为字节（供 Checkpoint.TaskGraphState）
func (g *TaskGraph) Marshal() ([]byte, error) {
	return json.Marshal(g)
}

// Unmarshal 从字节反序列化
func (g *TaskGraph) Unmarshal(data []byte) error {
	return json.Unmarshal(data, g)
}

// TaskResult 单节点执行结果（供 Executor 写回）
type TaskResult struct {
	NodeID  string
	Output  string
	Err     string
	At      time.Time
}
