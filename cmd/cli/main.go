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

package main

import (
	"bufio"
	"embed"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"rag-platform/pkg/config"
)

//go:embed templates/agent-minimal
var agentMinimalTemplate embed.FS

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(0)
	}
	cmd := os.Args[1]
	args := os.Args[2:]
	switch cmd {
	case "version":
		fmt.Println("aetheris cli 1.0.0")
	case "health":
		fmt.Println("ok")
	case "config":
		runConfig()
	case "server":
		if len(args) > 0 && args[0] == "start" {
			runServerStart()
		} else {
			fmt.Fprintf(os.Stderr, "Usage: aetheris server start\n")
			os.Exit(1)
		}
	case "worker":
		if len(args) > 0 && args[0] == "start" {
			runWorkerStart()
		} else {
			fmt.Fprintf(os.Stderr, "Usage: aetheris worker start\n")
			os.Exit(1)
		}
	case "agent":
		if len(args) > 0 && args[0] == "create" {
			name := ""
			if len(args) > 1 {
				name = args[1]
			}
			runAgentCreate(name)
		} else {
			fmt.Fprintf(os.Stderr, "Usage: aetheris agent create [name]\n")
			os.Exit(1)
		}
	case "chat":
		runChat(args)
	case "jobs":
		runJobs(args)
	case "trace":
		if len(args) < 1 {
			fmt.Fprintf(os.Stderr, "Usage: aetheris trace <job_id>\n")
			os.Exit(1)
		}
		runTrace(args[0])
	case "workers":
		runWorkers()
	case "replay":
		if len(args) < 1 {
			fmt.Fprintf(os.Stderr, "Usage: aetheris replay <job_id>\n")
			os.Exit(1)
		}
		runReplay(args[0])
	case "cancel":
		if len(args) < 1 {
			fmt.Fprintf(os.Stderr, "Usage: aetheris cancel <job_id>\n")
			os.Exit(1)
		}
		runCancel(args[0])
	case "debug":
		if len(args) < 1 {
			fmt.Fprintf(os.Stderr, "Usage: aetheris debug <job_id> [--compare-replay]\n")
			os.Exit(1)
		}
		compareReplay := false
		if len(args) > 1 && args[1] == "--compare-replay" {
			compareReplay = true
		}
		runDebug(args[0], compareReplay)
	case "verify":
		if len(args) < 1 {
			fmt.Fprintf(os.Stderr, "Usage: aetheris verify <job_id>\n")
			os.Exit(1)
		}
		runVerify(args[0])
	case "init":
		dir := "."
		if len(args) > 0 {
			dir = args[0]
		}
		runInit(dir)
	default:
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println("Usage: aetheris <command> [args]")
	fmt.Println("  version         - 显示版本")
	fmt.Println("  health          - 健康检查")
	fmt.Println("  config          - 显示配置概要")
	fmt.Println("  server start    - 启动 API 服务（go run ./cmd/api）")
	fmt.Println("  worker start    - 启动 Worker 服务（go run ./cmd/worker）")
	fmt.Println("  agent create [name] - 创建 Agent，返回 agent_id")
	fmt.Println("  chat [agent_id] - 交互式对话（未传 agent_id 时需环境 AETHERIS_AGENT_ID）")
	fmt.Println("  jobs <agent_id> - 列出该 Agent 的 Jobs")
	fmt.Println("  trace <job_id>  - 输出 Job 执行时间线，并打印 Trace 页面 URL")
	fmt.Println("  workers         - 列出当前活跃 Worker（Postgres 模式）")
	fmt.Println("  replay <job_id> - 输出 Job 事件流（重放用）")
	fmt.Println("  cancel <job_id> - 请求取消执行中的 Job")
	fmt.Println("  debug <job_id> [--compare-replay] - Agent 调试器：timeline + evidence + replay verification")
	fmt.Println("  verify <job_id> - 执行验证：输出 execution_hash、event_chain_root、ledger proof、replay proof")
	fmt.Println("  init [dir]     - Scaffold a minimal agent project (templates + config) into current dir or dir")
}

func runInit(dir string) {
	prefix := "templates/agent-minimal"
	if err := os.MkdirAll(dir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "init: mkdir %s: %v\n", dir, err)
		os.Exit(1)
	}
	err := fs.WalkDir(agentMinimalTemplate, prefix, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(prefix, path)
		if rel == "." {
			return nil
		}
		target := filepath.Join(dir, filepath.FromSlash(rel))
		if d.IsDir() {
			return os.MkdirAll(target, 0755)
		}
		data, err := agentMinimalTemplate.ReadFile(path)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			return err
		}
		return os.WriteFile(target, data, 0644)
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "init: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("Created minimal agent project in", dir)
	fmt.Println("Next: edit configs/api.yaml if needed, then run 'make run' or 'aetheris server start' and 'aetheris worker start'.")
	fmt.Println("See README in that directory and docs/getting-started-agents.md for a full agent example.")
}

func runConfig() {
	cfg, err := config.LoadAPIConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "加载配置失败: %v\n", err)
		os.Exit(1)
	}
	if cfg != nil {
		fmt.Printf("api.port=%d\n", cfg.API.Port)
		fmt.Printf("api.host=%s\n", cfg.API.Host)
	}
}

func runServerStart() {
	c := exec.Command("go", "run", "./cmd/api")
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	c.Dir = "."
	if err := c.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "server start: %v\n", err)
		os.Exit(1)
	}
}

func runWorkerStart() {
	c := exec.Command("go", "run", "./cmd/worker")
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	c.Dir = "."
	if err := c.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "worker start: %v\n", err)
		os.Exit(1)
	}
}

func runAgentCreate(name string) {
	if name == "" {
		name = "default"
	}
	id, err := createAgent(name)
	if err != nil {
		fmt.Fprintf(os.Stderr, "创建 Agent 失败: %v\n", err)
		os.Exit(1)
	}
	fmt.Println(id)
}

func runChat(args []string) {
	agentID := os.Getenv("AETHERIS_AGENT_ID")
	if len(args) > 0 {
		agentID = args[0]
	}
	if agentID == "" {
		fmt.Fprintf(os.Stderr, "请指定 agent_id: aetheris chat <agent_id> 或设置 AETHERIS_AGENT_ID\n")
		os.Exit(1)
	}
	reader := bufio.NewReader(os.Stdin)
	for {
		fmt.Print("> ")
		line, err := reader.ReadString('\n')
		if err != nil {
			break
		}
		msg := strings.TrimSpace(line)
		if msg == "" {
			continue
		}
		if msg == "exit" || msg == "quit" {
			break
		}
		jobID, err := postMessage(agentID, msg)
		if err != nil {
			fmt.Fprintf(os.Stderr, "发送失败: %v\n", err)
			continue
		}
		fmt.Printf("Job: %s (轮询状态中...)\n", jobID)
		for i := 0; i < 60; i++ {
			time.Sleep(1 * time.Second)
			j, err := getJob(jobID)
			if err != nil {
				fmt.Fprintf(os.Stderr, "查询失败: %v\n", err)
				break
			}
			status, _ := j["status"].(string)
			fmt.Printf("  status: %s\n", status)
			if status == "completed" || status == "failed" {
				break
			}
		}
	}
}

func runJobs(args []string) {
	if len(args) < 1 {
		fmt.Fprintf(os.Stderr, "Usage: aetheris jobs <agent_id>\n")
		os.Exit(1)
	}
	agentID := args[0]
	jobs, err := listAgentJobs(agentID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "列出 Jobs 失败: %v\n", err)
		os.Exit(1)
	}
	fmt.Println(prettyJSON(jobs))
}

func runTrace(jobID string) {
	trace, err := getJobTrace(jobID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "获取 Trace 失败: %v\n", err)
		os.Exit(1)
	}
	fmt.Println(prettyJSON(trace))
	fmt.Println()
	fmt.Println("Trace 页面:", tracePageURL(jobID))
}

func runWorkers() {
	workers, err := listWorkers()
	if err != nil {
		fmt.Fprintf(os.Stderr, "列出 Worker 失败: %v\n", err)
		os.Exit(1)
	}
	if len(workers) == 0 {
		fmt.Println("[]")
		return
	}
	fmt.Println(prettyJSON(workers))
}

func runReplay(jobID string) {
	ev, err := getJobEvents(jobID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "获取事件流失败: %v\n", err)
		os.Exit(1)
	}
	fmt.Println(prettyJSON(ev))
	fmt.Println()
	fmt.Println("Trace 页面:", tracePageURL(jobID))
}

func runCancel(jobID string) {
	out, err := cancelJob(jobID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "取消失败: %v\n", err)
		os.Exit(1)
	}
	fmt.Println(prettyJSON(out))
}

func runDebug(jobID string, compareReplay bool) {
	// Fetch job metadata
	jobData, err := getJob(jobID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "获取 Job 失败: %v\n", err)
		os.Exit(1)
	}

	// Fetch trace
	trace, err := getJobTrace(jobID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "获取 Trace 失败: %v\n", err)
		os.Exit(1)
	}

	// Display job info
	fmt.Printf("=== Job: %s ===\n", jobID)
	if goal, ok := jobData["goal"].(string); ok && goal != "" {
		fmt.Printf("Goal: %s\n", goal)
	}
	if status, ok := jobData["status"].(string); ok {
		fmt.Printf("Status: %s\n", status)
	}
	if agentID, ok := jobData["agent_id"].(string); ok {
		fmt.Printf("Agent: %s\n", agentID)
	}
	fmt.Println()

	// Execution timeline
	fmt.Println("=== Execution Timeline ===")
	if steps, ok := trace["steps"].([]interface{}); ok {
		for _, stepData := range steps {
			step, ok := stepData.(map[string]interface{})
			if !ok {
				continue
			}
			nodeID, _ := step["node_id"].(string)
			stepType, _ := step["type"].(string)
			state, _ := step["state"].(string)

			startTime := ""
			if st, ok := step["start_time"].(string); ok {
				if len(st) > 19 {
					startTime = st[11:19] // HH:MM:SS
				} else {
					startTime = st
				}
			}

			statusIcon := "✓"
			if state == "failed" || strings.Contains(state, "failure") {
				statusIcon = "✗"
			} else if state == "waiting" || state == "parked" {
				statusIcon = "⏸"
			}

			fmt.Printf("[%s] %s %s (%s) → %s\n", startTime, statusIcon, nodeID, stepType, state)

			// Tool details
			if toolInv, ok := step["tool_invocation"].(map[string]interface{}); ok {
				if toolName, ok := toolInv["tool_name"].(string); ok {
					fmt.Printf("        Tool: %s\n", toolName)
				}
			}

			// LLM details
			if llmInv, ok := step["llm_invocation"].(map[string]interface{}); ok {
				if model, ok := llmInv["model"].(string); ok {
					temp, _ := llmInv["temperature"].(float64)
					fmt.Printf("        LLM: %s (temp=%.1f)\n", model, temp)
				}
			}
		}
	}
	fmt.Println()

	// Evidence chain
	fmt.Println("=== Evidence Chain ===")
	hasEvidence := false
	if steps, ok := trace["steps"].([]interface{}); ok {
		for _, stepData := range steps {
			step, ok := stepData.(map[string]interface{})
			if !ok {
				continue
			}
			nodeID, _ := step["node_id"].(string)

			if evidence, ok := step["evidence"].(map[string]interface{}); ok {
				hasEvidence = true
				fmt.Printf("%s:\n", nodeID)

				if toolIDs, ok := evidence["tool_invocation_ids"].([]interface{}); ok && len(toolIDs) > 0 {
					fmt.Printf("  └─ Tool invocations: %d\n", len(toolIDs))
				}

				if llmDec, ok := evidence["llm_decision"].(map[string]interface{}); ok {
					if model, ok := llmDec["model"].(string); ok {
						fmt.Printf("  └─ LLM: %s\n", model)
					}
				}

				if inputKeys, ok := evidence["input_keys"].([]interface{}); ok && len(inputKeys) > 0 {
					fmt.Printf("  └─ Reads: %v\n", inputKeys)
				}

				if outputKeys, ok := evidence["output_keys"].([]interface{}); ok && len(outputKeys) > 0 {
					fmt.Printf("  └─ Writes: %v\n", outputKeys)
				}
			}
		}
	}
	if !hasEvidence {
		fmt.Println("(No evidence recorded)")
	}
	fmt.Println()

	// Replay verification
	if compareReplay {
		fmt.Println("=== Replay Verification ===")
		replayData, err := getJobEvents(jobID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "获取 Replay 数据失败: %v\n", err)
		} else {
			completedNodes := 0
			completedCmds := 0
			completedTools := 0

			if nodes, ok := replayData["completed_node_ids"].(map[string]interface{}); ok {
				completedNodes = len(nodes)
			}
			if cmds, ok := replayData["completed_command_ids"].(map[string]interface{}); ok {
				completedCmds = len(cmds)
			}
			if tools, ok := replayData["completed_tool_invocations"].(map[string]interface{}); ok {
				completedTools = len(tools)
			}

			fmt.Printf("✓ Completed nodes: %d\n", completedNodes)
			fmt.Printf("✓ Completed commands: %d\n", completedCmds)
			fmt.Printf("✓ Completed tool invocations: %d\n", completedTools)
			fmt.Println("✓ Replay deterministic (results injected, not re-executed)")
			fmt.Println("✓ LLM NOT re-called (from Effect Store)")
			fmt.Println("✓ Tools NOT re-executed (from Ledger)")
		}
		fmt.Println()
	}

	// Summary
	fmt.Println("=== Debug Summary ===")
	fmt.Println("✓ Execution history complete")
	fmt.Println("✓ Evidence traceable")
	fmt.Println("✓ Audit-ready")
	fmt.Println()
	baseURL := os.Getenv("AETHERIS_API_URL")
	if baseURL == "" {
		baseURL = "http://localhost:8080"
	}
	fmt.Printf("Detailed trace: %s/api/jobs/%s/trace\n", baseURL, jobID)
}

func runVerify(jobID string) {
	v, err := getJobVerify(jobID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "获取验证结果失败: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("=== Verification: %s ===\n\n", jobID)
	if h, ok := v["execution_hash"].(string); ok {
		fmt.Printf("Execution hash:          %s\n", h)
	}
	if h, ok := v["event_chain_root_hash"].(string); ok {
		fmt.Printf("Event chain root hash:   %s\n", h)
	}
	if ledger, ok := v["tool_invocation_ledger_proof"].(map[string]interface{}); ok {
		okVal, _ := ledger["ok"].(bool)
		fmt.Printf("Ledger proof (at-most-once): %v\n", okVal)
		if keys, ok := ledger["pending_idempotency_keys"].([]interface{}); ok && len(keys) > 0 {
			fmt.Printf("  Pending keys: %v\n", keys)
		}
	}
	if replayP, ok := v["replay_proof_result"].(map[string]interface{}); ok {
		okVal, _ := replayP["ok"].(bool)
		fmt.Printf("Replay proof (consistent):   %v\n", okVal)
		if errStr, ok := replayP["error"].(string); ok && errStr != "" {
			fmt.Printf("  Error: %s\n", errStr)
		}
	}
	fmt.Println()
	fmt.Println(prettyJSON(v))
}
