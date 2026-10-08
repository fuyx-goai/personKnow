// main.go —— 进程入口（网关进程）：装配所有层，启动 Gin HTTP 服务
//
// 这是全项目唯一的"装配车间"：model 层不依赖任何人，repo 层实现 model 层的端口，
// service 层编排业务用例，gateway 层负责对外。所有依赖在这里被"接起来"（依赖注入）。
//
// 启动：
//
//	go run ./cmd/gateway
//	# 或 make run
//
// 配置：configs/config.yaml（读取逻辑见 pkg/config/config.go）。
// 想用不入库的私密配置：
//
//	CONFIG_FILE=configs/config.local.yaml go run ./cmd/gateway
//
// 前端界面（已编译进二进制，浏览器直接打开服务地址即可）：
//
//	http://localhost:8080
//
// 接口一览：
//
//	GET    /api/health        健康检查
//	GET    /api/config        查看当前生效的配置（Key 已打码）
//	GET    /api/stats         知识库统计（向量库类型 + 片段数）
//	GET    /api/chunks        列出全部片段（藏书页）
//	DELETE /api/chunks        删除片段（?source=xxx）或清空整库
//	POST   /api/ingest        摄入文档 {"path":"./docs"}
//	POST   /api/chat          问答（非流式）{"question":"..."}
//	POST   /api/chat/stream   问答（流式 SSE）{"question":"..."}
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"knowledge-base/pkg/config"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		fatal("加载配置失败", err)
	}
	if err := cfg.Validate(); err != nil {
		fatal("配置校验失败", err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	runtime, err := buildRuntime(ctx, cfg)
	if err != nil {
		fatal("初始化服务失败", err)
	}
	defer runtime.Close()
	fmt.Println(cfg.Describe())
	printUsage(cfg)
	if err := runtime.Run(ctx, 15*time.Second); err != nil {
		fatal("服务运行失败", err)
	}
}

// printUsage 打印接口用法，省得每次都翻文档
func printUsage(cfg config.Config) {
	fmt.Printf(`知识库界面：http://localhost%s

接口：
  GET    /api/health          健康检查
  GET    /api/config          查看当前配置
  GET    /api/stats           知识库统计
  GET    /api/chunks          列出片段    curl 'localhost%s/api/chunks?limit=50'
  DELETE /api/chunks          删除片段    curl -X DELETE 'localhost%s/api/chunks?source=go-notes.md'（不带 source 则清空整库）
  POST   /api/ingest          摄入文档    curl -X POST localhost%s/api/ingest -H 'Content-Type: application/json' -d '{"path":"./docs"}'
  POST   /api/chat            问答        curl -X POST localhost%s/api/chat -H 'Content-Type: application/json' -d '{"question":"goroutine 是什么"}'
  POST   /api/chat/stream     流式问答    curl -N -X POST localhost%s/api/chat/stream -H 'Content-Type: application/json' -d '{"question":"goroutine 是什么"}'

完整接口：/api/v1（微信登录、多知识库、文件、索引、问答、用量与审计）

`, cfg.HTTPAddr, cfg.HTTPAddr, cfg.HTTPAddr, cfg.HTTPAddr, cfg.HTTPAddr, cfg.HTTPAddr)
}

// fatal 打印人话错误并退出（教学项目不甩 panic 堆栈，对新手更友好）
func fatal(msg string, err error) {
	fmt.Printf("✗ %s: %v\n", msg, err)
	os.Exit(1)
}
