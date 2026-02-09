package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"rag-platform/pkg/config"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(0)
	}
	cmd := os.Args[1]
	args := os.Args[2:]
	switch cmd {
	case "version":
		fmt.Println("rag-platform cli 0.1.0")
	case "health":
		fmt.Println("ok")
	case "config":
		runConfig()
	case "server":
		if len(args) > 0 && args[0] == "start" {
			runServerStart()
		} else {
			fmt.Fprintf(os.Stderr, "Usage: corag server start\n")
			os.Exit(1)
		}
	case "worker":
		if len(args) > 0 && args[0] == "start" {
			runWorkerStart()
		} else {
			fmt.Fprintf(os.Stderr, "Usage: corag worker start\n")
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
			fmt.Fprintf(os.Stderr, "Usage: corag agent create [name]\n")
			os.Exit(1)
		}
	case "chat":
		runChat(args)
	case "jobs":
		runJobs(args)
	case "trace":
		if len(args) < 1 {
			fmt.Fprintf(os.Stderr, "Usage: corag trace <job_id>\n")
			os.Exit(1)
		}
		runTrace(args[0])
	case "workers":
		runWorkers()
	case "replay":
		if len(args) < 1 {
			fmt.Fprintf(os.Stderr, "Usage: corag replay <job_id>\n")
			os.Exit(1)
		}
		runReplay(args[0])
	case "cancel":
		if len(args) < 1 {
			fmt.Fprintf(os.Stderr, "Usage: corag cancel <job_id>\n")
			os.Exit(1)
		}
		runCancel(args[0])
	default:
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println("Usage: corag <command> [args]")
	fmt.Println("  version         - 显示版本")
	fmt.Println("  health          - 健康检查")
	fmt.Println("  config          - 显示配置概要")
	fmt.Println("  server start    - 启动 API 服务（go run ./cmd/api）")
	fmt.Println("  worker start    - 启动 Worker 服务（go run ./cmd/worker）")
	fmt.Println("  agent create [name] - 创建 Agent，返回 agent_id")
	fmt.Println("  chat [agent_id] - 交互式对话（未传 agent_id 时需环境 CORAG_AGENT_ID）")
	fmt.Println("  jobs <agent_id> - 列出该 Agent 的 Jobs")
	fmt.Println("  trace <job_id>  - 输出 Job 执行时间线，并打印 Trace 页面 URL")
	fmt.Println("  workers         - 列出当前活跃 Worker（Postgres 模式）")
	fmt.Println("  replay <job_id> - 输出 Job 事件流（重放用）")
	fmt.Println("  cancel <job_id> - 请求取消执行中的 Job")
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
	agentID := os.Getenv("CORAG_AGENT_ID")
	if len(args) > 0 {
		agentID = args[0]
	}
	if agentID == "" {
		fmt.Fprintf(os.Stderr, "请指定 agent_id: corag chat <agent_id> 或设置 CORAG_AGENT_ID\n")
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
		fmt.Fprintf(os.Stderr, "Usage: corag jobs <agent_id>\n")
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
