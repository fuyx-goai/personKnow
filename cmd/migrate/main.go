package main

import (
	"context"
	"fmt"
	"log"
	"os"

	platformpostgres "knowledge-base/internal/platform/postgres"
	"knowledge-base/pkg/config"
)

func main() {
	if err := run(context.Background(), os.Args[1:]); err != nil {
		log.Fatal(err)
	}
}

func run(ctx context.Context, args []string) error {
	command := "status"
	if len(args) > 0 {
		command = args[0]
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if cfg.Database.URL == "" {
		return fmt.Errorf("DATABASE_URL 不能为空")
	}
	pool, err := platformpostgres.Open(ctx, cfg.Database)
	if err != nil {
		return err
	}
	defer pool.Close()

	switch command {
	case "up":
		return platformpostgres.Migrate(ctx, pool)
	case "status":
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
	default:
		return fmt.Errorf("未知命令 %q，仅支持 up/status", command)
	}
}
