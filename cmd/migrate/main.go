package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	document "knowledge-base/internal/document/repo"
	"knowledge-base/internal/knowledge/repo/vectorstore"
	migration "knowledge-base/internal/migration/entity"
	migrationrepo "knowledge-base/internal/migration/repo"
	migrationservice "knowledge-base/internal/migration/service"
	platformpostgres "knowledge-base/internal/platform/postgres"
	platformvector "knowledge-base/internal/platform/vector"
	"knowledge-base/pkg/config"
)

type commandOptions struct {
	name       string
	targetUser uuid.UUID
	source     string
	backupDir  string
}

type lazyVectorWriter struct {
	build  func() (platformvector.Repository, error)
	writer platformvector.Repository
}

func main() {
	if err := run(context.Background(), os.Args[1:]); err != nil {
		log.Fatal(err)
	}
}

func run(ctx context.Context, args []string) error {
	options, err := parseCommand(args)
	if err != nil {
		return err
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if cfg.Database.URL == "" {
		return fmt.Errorf("配置文件 database.url 不能为空")
	}
	pool, err := platformpostgres.Open(ctx, cfg.Database)
	if err != nil {
		return err
	}
	defer pool.Close()

	switch options.name {
	case "up":
		return platformpostgres.Migrate(ctx, pool)
	case "status":
		return printStatus(ctx, pool)
	case "legacy":
		return runLegacy(ctx, cfg, pool, options)
	default:
		return fmt.Errorf("未知命令 %q，仅支持 up/status/legacy", options.name)
	}
}

func parseCommand(args []string) (commandOptions, error) {
	if len(args) == 0 {
		return commandOptions{name: "status"}, nil
	}
	options := commandOptions{name: args[0]}
	if options.name == "up" || options.name == "status" {
		if len(args) != 1 {
			return commandOptions{}, fmt.Errorf("%s 命令不接受额外参数", options.name)
		}
		return options, nil
	}
	if options.name != "legacy" {
		return commandOptions{}, fmt.Errorf("未知命令 %q，仅支持 up/status/legacy", options.name)
	}
	flags := flag.NewFlagSet("legacy", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	targetUser := flags.String("target-user", "", "目标用户 UUID")
	flags.StringVar(&options.source, "source", "", "旧 knowledge.json 路径")
	flags.StringVar(&options.backupDir, "backup-dir", "", "备份目录")
	if err := flags.Parse(args[1:]); err != nil {
		return commandOptions{}, err
	}
	if flags.NArg() != 0 || strings.TrimSpace(options.source) == "" || strings.TrimSpace(options.backupDir) == "" {
		return commandOptions{}, errors.New("legacy 命令要求 --target-user、--source 和 --backup-dir")
	}
	userID, err := uuid.Parse(strings.TrimSpace(*targetUser))
	if err != nil {
		return commandOptions{}, errors.New("--target-user 必须是有效 UUID")
	}
	options.targetUser = userID
	return options, nil
}

func printStatus(ctx context.Context, pool *pgxpool.Pool) error {
	statuses, err := platformpostgres.Status(ctx, pool)
	if err != nil {
		return err
	}
	for _, status := range statuses {
		state := "pending"
		if status.AppliedAt != nil {
			state = "applied"
		}
		fmt.Printf("%03d %-8s %s\n", status.Version, state, status.Name)
	}
	return nil
}

func runLegacy(ctx context.Context, cfg config.Config, pool *pgxpool.Pool, options commandOptions) error {
	if err := platformpostgres.Migrate(ctx, pool); err != nil {
		return err
	}
	sourcePath, err := filepath.Abs(options.source)
	if err != nil {
		return err
	}
	writer := &lazyVectorWriter{build: func() (platformvector.Repository, error) {
		store, err := migrationVectorStore(ctx, cfg, sourcePath)
		if err != nil {
			return nil, err
		}
		return vectorstore.NewScopedRepository(store), nil
	}}
	migrator := migrationservice.NewLegacyMigrator(migration.Dependencies{
		Repository: migrationrepo.NewPostgresRepository(pool), Vectors: writer,
		Files: document.NewLocalFileStore(cfg.Storage.DataDir),
	})
	report, err := migrator.Migrate(ctx, migration.Options{
		TargetUser: options.targetUser, SourcePath: sourcePath, BackupDir: options.backupDir,
	})
	if err != nil {
		return err
	}
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(report); err != nil {
		return err
	}
	if len(report.Failures) > 0 {
		return fmt.Errorf("迁移完成但有 %d 项失败，请根据错误码处理后重试", len(report.Failures))
	}
	return nil
}

func migrationVectorStore(ctx context.Context, cfg config.Config, sourcePath string) (vectorstore.VectorStore, error) {
	switch cfg.VectorStore {
	case config.StoreMem, "":
		return vectorstore.NewMemStoreAt(ctx, nil, sourcePath)
	case config.StoreMilvus:
		return vectorstore.NewMilvusStore(ctx, cfg.Milvus, nil)
	default:
		return nil, fmt.Errorf("未知的向量库类型 %q", cfg.VectorStore)
	}
}

func (writer *lazyVectorWriter) Upsert(ctx context.Context, chunks []platformvector.Chunk) error {
	if writer.writer == nil {
		created, err := writer.build()
		if err != nil {
			return err
		}
		writer.writer = created
	}
	return writer.writer.Upsert(ctx, chunks)
}
