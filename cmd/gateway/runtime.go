package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	accounthandler "knowledge-base/internal/account/handler"
	accountrepo "knowledge-base/internal/account/repo"
	accountservice "knowledge-base/internal/account/service"
	chathandler "knowledge-base/internal/chat/handler"
	chatrepo "knowledge-base/internal/chat/repo"
	chatservice "knowledge-base/internal/chat/service"
	documenthandler "knowledge-base/internal/document/handler"
	documentrepo "knowledge-base/internal/document/repo"
	documentservice "knowledge-base/internal/document/service"
	legacyhandler "knowledge-base/internal/gateway/handler"
	"knowledge-base/internal/gateway/router"
	indexinghandler "knowledge-base/internal/indexing/handler"
	indexingrepo "knowledge-base/internal/indexing/repo"
	indexingservice "knowledge-base/internal/indexing/service"
	legacyrepo "knowledge-base/internal/knowledge/repo"
	"knowledge-base/internal/knowledge/repo/vectorstore"
	legacyservice "knowledge-base/internal/knowledge/service"
	libraryhandler "knowledge-base/internal/library/handler"
	libraryrepo "knowledge-base/internal/library/repo"
	libraryservice "knowledge-base/internal/library/service"
	"knowledge-base/internal/platform/logging"
	platformpostgres "knowledge-base/internal/platform/postgres"
	usagehandler "knowledge-base/internal/usage/handler"
	usagerepo "knowledge-base/internal/usage/repo"
	usageservice "knowledge-base/internal/usage/service"
	"knowledge-base/pkg/config"
)

type applicationRuntime struct {
	server *http.Server
	worker *indexingservice.Worker
	pool   *pgxpool.Pool
	cfg    config.Config
}

// buildRuntime 创建日志、数据库连接和业务依赖，并在监听端口前完成 migration。
// 任一步失败都会关闭已创建的资源，避免启动失败后残留连接。
func buildRuntime(ctx context.Context, cfg config.Config) (*applicationRuntime, error) {
	logger, err := logging.NewFile(cfg.Log)
	if err != nil {
		return nil, err
	}
	slog.SetDefault(logger)
	if !cfg.DatabaseEnabled() {
		engine, err := composeStandaloneApplication(ctx, cfg)
		if err != nil {
			return nil, err
		}
		slog.Warn("database_disabled", "mode", "standalone", "reason", "database.url is empty")
		server := &http.Server{Addr: cfg.HTTPAddr, Handler: engine, ReadHeaderTimeout: 10 * time.Second}
		return &applicationRuntime{server: server, cfg: cfg}, nil
	}
	pool, err := platformpostgres.Open(ctx, cfg.Database)
	if err != nil {
		return nil, err
	}
	if err := platformpostgres.Migrate(ctx, pool); err != nil {
		pool.Close()
		return nil, err
	}
	engine, worker, err := composeApplication(ctx, cfg, pool)
	if err != nil {
		pool.Close()
		return nil, err
	}
	server := &http.Server{Addr: cfg.HTTPAddr, Handler: engine, ReadHeaderTimeout: 10 * time.Second}
	return &applicationRuntime{server: server, worker: worker, pool: pool, cfg: cfg}, nil
}

// composeStandaloneApplication 只装配基础 RAG 接口，用于尚未配置 PostgreSQL 的本地启动阶段。
// 账号、多知识库、文件任务和审计都需要关系数据，因此不会用临时假数据伪装成可用。
func composeStandaloneApplication(ctx context.Context, cfg config.Config) (http.Handler, error) {
	chatModel, err := legacyrepo.NewChatModel(ctx, cfg.LLM)
	if err != nil {
		return nil, err
	}
	embedder, err := legacyrepo.NewEmbedder(ctx, cfg.LLM)
	if err != nil {
		return nil, err
	}
	store, err := vectorstore.NewVectorStore(ctx, cfg, embedder)
	if err != nil {
		return nil, err
	}
	repository := vectorstore.NewRepository(store)
	ingest, err := legacyrepo.NewIngestPipeline(ctx, embedder, repository)
	if err != nil {
		return nil, err
	}
	chat, err := legacyrepo.NewChatPipeline(ctx, chatModel, repository)
	if err != nil {
		return nil, err
	}
	handler := legacyhandler.New(cfg, legacyservice.NewChatService(chat),
		legacyservice.NewIngestService(ingest), legacyservice.NewLibraryService(repository))
	return router.New(handler), nil
}

// composeApplication 是全栈应用的装配入口：复用底层模型与向量库，
// 再把账号、知识库、文档、索引、问答和用量模块连接到同一套路由。
func composeApplication(ctx context.Context, cfg config.Config, pool *pgxpool.Pool) (http.Handler, *indexingservice.Worker, error) {
	chatModel, err := legacyrepo.NewChatModel(ctx, cfg.LLM)
	if err != nil {
		return nil, nil, err
	}
	embedder, err := legacyrepo.NewEmbedder(ctx, cfg.LLM)
	if err != nil {
		return nil, nil, err
	}
	store, err := vectorstore.NewVectorStore(ctx, cfg, embedder)
	if err != nil {
		return nil, nil, err
	}
	legacyRepository := vectorstore.NewRepository(store)
	ingestPipeline, err := legacyrepo.NewIngestPipeline(ctx, embedder, legacyRepository)
	if err != nil {
		return nil, nil, err
	}
	legacyChat, err := legacyrepo.NewChatPipeline(ctx, chatModel, legacyRepository)
	if err != nil {
		return nil, nil, err
	}
	legacy := legacyhandler.New(cfg, legacyservice.NewChatService(legacyChat),
		legacyservice.NewIngestService(ingestPipeline), legacyservice.NewLibraryService(legacyRepository))

	usageRepository := usagerepo.NewPostgresRepository(pool)
	usageService := usageservice.NewService(usageRepository, time.Now)
	indexRepository := indexingrepo.NewPostgresRepository(pool)
	libraryService := libraryservice.NewService(libraryrepo.NewPostgresRepository(pool), indexRepository).UseAuditor(usageService)
	files := documentrepo.NewLocalFileStore(cfg.Storage.DataDir)
	documentService := documentservice.NewService(documentservice.Dependencies{
		Repository: documentrepo.NewPostgresRepository(pool), Libraries: libraryService, Files: files,
		Quota: usageService, Jobs: indexRepository, Auditor: usageService, MaxBytes: cfg.Storage.MaxFileBytes,
	})
	codec, err := accountrepo.NewIdentityCodec([]byte(cfg.Auth.IdentitySecret))
	if err != nil {
		return nil, nil, err
	}
	tokens := accountservice.NewTokenManager([]byte(cfg.Auth.JWTSecret), cfg.Auth.AccessTTL, cfg.Auth.RefreshTTL)
	accountService := accountservice.NewService(accountrepo.NewPostgresRepository(pool, codec),
		accountrepo.NewHTTPWeChatClient(cfg.Auth.WeChatAppID, cfg.Auth.WeChatSecret, nil), tokens, time.Now).UseAuditor(usageService)
	scopedVectors := vectorstore.NewScopedRepository(store)
	sourceLoader := indexingservice.NewPostgresSourceLoader(pool, files, documentrepo.NewParserRegistry(), usageService)
	processor := indexingservice.NewProcessor(indexingservice.ProcessorDependencies{
		Sources: sourceLoader, Embedder: embedder, Vectors: scopedVectors, Quota: usageService,
	})
	worker := indexingservice.NewWorker("gateway-"+uuid.NewString(), indexRepository, processor, cfg.Worker.Lease, time.Now)
	chatRepository := chatrepo.NewPostgresRepository(pool)
	chatService := chatservice.NewService(chatservice.ServiceDependencies{
		Repository: chatRepository, Engine: chatservice.NewVectorEngine(scopedVectors, chatservice.NewEinoGenerator(chatModel)),
		Quota: usageService, Now: time.Now,
	})
	engine := router.New(legacy, router.V1Handlers{
		Verifier: tokens, Account: accounthandler.NewHandler(accountService), Library: libraryhandler.NewHandler(libraryService),
		Document: documenthandler.NewHandler(documentService), Indexing: indexinghandler.NewHandler(indexRepository),
		Chat: chathandler.NewHandler(chatService), Usage: usagehandler.NewHandler(usageService),
	})
	return engine, worker, nil
}

// Run 同时运行 HTTP 服务和索引 Worker；任一组件失败或收到退出信号时统一收口。
func (runtime *applicationRuntime) Run(ctx context.Context, shutdownTimeout time.Duration) error {
	errorsChannel := make(chan error, 2)
	workerContext, cancelWorker := context.WithCancel(ctx)
	defer cancelWorker()
	if runtime.worker != nil {
		go func() {
			if err := runtime.worker.Run(workerContext, runtime.cfg.Worker.Concurrency, runtime.cfg.Worker.Poll); err != nil {
				errorsChannel <- err
			}
		}()
	}
	go func() {
		if err := runtime.server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errorsChannel <- err
		}
	}()
	select {
	case err := <-errorsChannel:
		return err
	case <-ctx.Done():
		cancelWorker()
		shutdownContext, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		return runtime.server.Shutdown(shutdownContext)
	}
}

// Close 释放进程持有的数据库连接池，可安全重复用于启动失败和正常退出路径。
func (runtime *applicationRuntime) Close() {
	if runtime.pool != nil {
		runtime.pool.Close()
	}
}
