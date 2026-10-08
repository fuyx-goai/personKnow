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

	"knowledge-base/internal/gateway/handler"
	"knowledge-base/internal/gateway/router"
	"knowledge-base/internal/knowledge/repo"
	"knowledge-base/internal/knowledge/repo/vectorstore"
	"knowledge-base/internal/knowledge/service"
	"knowledge-base/pkg/config"
)

func main() {
	ctx := context.Background()

	// 1. 读取 configs/config.yaml 并打印摘要（确认豆包 Key、向量库选型是否符合预期）
	cfg, err := config.Load()
	if err != nil {
		fatal("加载配置失败", err)
	}
	fmt.Println(cfg.Describe())

	// 2. 创建 Eino 组件：对话模型、向量化模型
	cm, err := repo.NewChatModel(ctx, cfg.LLM)
	if err != nil {
		fatal("初始化对话模型失败", err)
	}
	emb, err := repo.NewEmbedder(ctx, cfg.LLM)
	if err != nil {
		fatal("初始化向量化模型失败", err)
	}

	// 3. 向量库：工厂按配置挑实现（mem / milvus），再适配成领域端口
	store, err := vectorstore.NewVectorStore(ctx, cfg, emb)
	if err != nil {
		fatal("初始化向量库失败", err)
	}
	knowledgeRepo := vectorstore.NewRepository(store)

	// 4. 构建两条 Eino 链（只编译一次，之后每个请求复用）
	ingestPipe, err := repo.NewIngestPipeline(ctx, emb, knowledgeRepo)
	if err != nil {
		fatal("构建摄入管道失败", err)
	}
	chatPipe, err := repo.NewChatPipeline(ctx, cm, knowledgeRepo)
	if err != nil {
		fatal("构建问答管道失败", err)
	}

	// 5. 组装 service 用例与 gateway 层
	h := handler.New(cfg,
		service.NewChatService(chatPipe),
		service.NewIngestService(ingestPipe),
		service.NewLibraryService(knowledgeRepo),
	)
	engine := router.New(h)

	// 6. 启动 HTTP 服务（阻塞运行）
	printUsage(cfg)
	if err := engine.Run(cfg.HTTPAddr); err != nil {
		fatal("启动 HTTP 服务失败", err)
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

`, cfg.HTTPAddr, cfg.HTTPAddr, cfg.HTTPAddr, cfg.HTTPAddr, cfg.HTTPAddr, cfg.HTTPAddr)
}

// fatal 打印人话错误并退出（教学项目不甩 panic 堆栈，对新手更友好）
func fatal(msg string, err error) {
	fmt.Printf("✗ %s: %v\n", msg, err)
	os.Exit(1)
}
