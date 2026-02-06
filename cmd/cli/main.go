package main

import (
	"fmt"
	"os"

	"rag-platform/pkg/config"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(0)
	}
	switch os.Args[1] {
	case "version":
		fmt.Println("rag-platform cli 0.1.0")
	case "health":
		// 仅做占位：可后续调用 app 提供的健康检查
		fmt.Println("ok")
	case "config":
		// 加载并打印当前配置路径/概要（不写 Pipeline/Workflow）
		cfg, err := config.LoadAPIConfig()
		if err != nil {
			fmt.Fprintf(os.Stderr, "加载配置失败: %v\n", err)
			os.Exit(1)
		}
		if cfg != nil {
			fmt.Printf("api.port=%d\n", cfg.API.Port)
		}
	default:
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println("Usage: cli <command>")
	fmt.Println("  version   - 显示版本")
	fmt.Println("  health    - 健康检查占位")
	fmt.Println("  config    - 显示配置概要")
}
