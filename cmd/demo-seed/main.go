package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/google/uuid"

	demorepo "knowledge-base/internal/demo/repo"
	demoservice "knowledge-base/internal/demo/service"
	platformpostgres "knowledge-base/internal/platform/postgres"
	"knowledge-base/pkg/config"
)

// demo-seed 只用于本地开发，将原型展示数据写入当前真实微信用户。
func main() {
	userID, err := run(context.Background(), os.Args[1:])
	if err != nil {
		fmt.Fprintf(os.Stderr, "演示数据初始化失败：%v\n", err)
		os.Exit(1)
	}
	fmt.Printf("演示数据已写入用户 %s\n", userID)
}

func run(ctx context.Context, args []string) (uuid.UUID, error) {
	flags := flag.NewFlagSet("demo-seed", flag.ContinueOnError)
	userText := flags.String("user", "", "目标用户 UUID；不填则使用最近登录的用户")
	if err := flags.Parse(args); err != nil {
		return uuid.Nil, err
	}
	cfg, err := config.Load()
	if err != nil {
		return uuid.Nil, err
	}
	if !cfg.DatabaseEnabled() {
		return uuid.Nil, fmt.Errorf("配置文件 database.url 为空，演示数据需要 PostgreSQL 完整模式")
	}
	pool, err := platformpostgres.Open(ctx, cfg.Database)
	if err != nil {
		return uuid.Nil, err
	}
	defer pool.Close()
	if err := platformpostgres.Migrate(ctx, pool); err != nil {
		return uuid.Nil, err
	}
	target := uuid.Nil
	if *userText != "" {
		target, err = uuid.Parse(*userText)
		if err != nil {
			return uuid.Nil, fmt.Errorf("--user 必须是有效 UUID：%w", err)
		}
	}
	seedService := demoservice.New(demorepo.NewPostgresRepository(pool)).WithContentFetcher(demoservice.NewContentFetcher(nil))
	report, err := seedService.Seed(ctx, target, cfg.Storage.DataDir)
	if err != nil {
		return uuid.Nil, err
	}
	fmt.Printf("知识库 %d 个，文档 %d 份，会话 %d 个，用量记录 %d 条\n", report.Libraries, report.Documents, report.Sessions, report.UsageRecords)
	return report.UserID, nil
}
