package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"knowledge-base/internal/account"
	"knowledge-base/internal/chat"
	"knowledge-base/internal/document"
	legacyhandler "knowledge-base/internal/gateway/handler"
	"knowledge-base/internal/gateway/router"
	"knowledge-base/internal/indexing"
	legacyrepo "knowledge-base/internal/knowledge/repo"
	"knowledge-base/internal/knowledge/repo/vectorstore"
	legacyservice "knowledge-base/internal/knowledge/service"
	"knowledge-base/internal/library"
	"knowledge-base/internal/platform/logging"
	platformpostgres "knowledge-base/internal/platform/postgres"
	"knowledge-base/internal/usage"
	"knowledge-base/pkg/config"
)

type applicationRuntime struct {
	server *http.Server
	worker *indexing.Worker
	pool   *pgxpool.Pool
	cfg    config.Config
}

func buildRuntime(ctx context.Context, cfg config.Config) (*applicationRuntime, error) {
	logger, err := logging.NewFile(cfg.Log)
	if err != nil {
		return nil, err
	}
	slog.SetDefault(logger)
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

func composeApplication(ctx context.Context, cfg config.Config, pool *pgxpool.Pool) (http.Handler, *indexing.Worker, error) {
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

	usageRepository := usage.NewPostgresRepository(pool)
	usageService := usage.NewService(usageRepository, time.Now)
	indexRepository := indexing.NewPostgresRepository(pool)
	libraryService := library.NewService(library.NewPostgresRepository(pool), indexRepository).UseAuditor(usageService)
	files := document.NewLocalFileStore(cfg.Storage.DataDir)
	documentService := document.NewService(document.Dependencies{
		Repository: document.NewPostgresRepository(pool), Libraries: libraryService, Files: files,
		Quota: usageService, Jobs: indexRepository, Auditor: usageService, MaxBytes: cfg.Storage.MaxFileBytes,
	})
	codec, err := account.NewIdentityCodec([]byte(cfg.Auth.IdentitySecret))
	if err != nil {
		return nil, nil, err
	}
	tokens := account.NewTokenManager([]byte(cfg.Auth.JWTSecret), cfg.Auth.AccessTTL, cfg.Auth.RefreshTTL)
	accountService := account.NewService(account.NewPostgresRepository(pool, codec),
		account.NewHTTPWeChatClient(cfg.Auth.WeChatAppID, cfg.Auth.WeChatSecret, nil), tokens, time.Now).UseAuditor(usageService)
	scopedVectors := vectorstore.NewScopedRepository(store)
	sourceLoader := indexing.NewPostgresSourceLoader(pool, files, document.NewParserRegistry(), usageService)
	processor := indexing.NewProcessor(indexing.ProcessorDependencies{
		Sources: sourceLoader, Embedder: embedder, Vectors: scopedVectors, Quota: usageService,
	})
	worker := indexing.NewWorker("gateway-"+uuid.NewString(), indexRepository, processor, cfg.Worker.Lease, time.Now)
	chatRepository := chat.NewPostgresRepository(pool)
	chatService := chat.NewService(chat.ServiceDependencies{
		Repository: chatRepository, Engine: chat.NewVectorEngine(scopedVectors, chat.NewEinoGenerator(chatModel)),
		Quota: usageService, Now: time.Now,
	})
	engine := router.New(legacy, router.V1Handlers{
		Verifier: tokens, Account: account.NewHandler(accountService), Library: library.NewHandler(libraryService),
		Document: document.NewHandler(documentService), Indexing: indexing.NewHandler(indexRepository),
		Chat: chat.NewHandler(chatService), Usage: usage.NewHandler(usageService),
	})
	return engine, worker, nil
}

func (runtime *applicationRuntime) Run(ctx context.Context, shutdownTimeout time.Duration) error {
	errorsChannel := make(chan error, 2)
	workerContext, cancelWorker := context.WithCancel(ctx)
	defer cancelWorker()
	go func() {
		if err := runtime.worker.Run(workerContext, runtime.cfg.Worker.Concurrency, runtime.cfg.Worker.Poll); err != nil {
			errorsChannel <- err
		}
	}()
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

func (runtime *applicationRuntime) Close() {
	if runtime.pool != nil {
		runtime.pool.Close()
	}
}
