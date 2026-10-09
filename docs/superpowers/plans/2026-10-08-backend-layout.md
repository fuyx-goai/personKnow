# Backend Layout Refactor Implementation Plan

> **For agentic workers:** Follow `AGENTS.md` for the directory contract. This plan is executed inline with verification after the mechanical moves.

**Goal:** 将后端业务包重排为参考 bw-cli 的 domain/layer 结构，并把目录约束写入项目规则。

**Architecture:** `internal/gateway` 负责协议入口和路由；每个业务域使用 `entity/handler/repo/service`；`internal/platform` 只承载基础设施；`cmd` 只负责装配和生命周期。

**Verification:** `gofmt -w`、`go test ./...`、`go vet ./...`，并检查 `internal/<domain>` 根目录不再有 Go 实现文件。

## 迁移映射

- `handler.go` -> `handler/handler.go`
- `model.go` 与业务命令 -> `entity/`
- `service.go` 与业务编排 -> `service/`
- `*_repository.go` 与持久化实现 -> `repo/`
- 领域端口/接口 -> `entity/`
- 不适用的 `dto`/`model` 层不创建空目录

迁移只改变包路径和目录职责，不改变 HTTP 路由、数据库 schema、JSON 字段或业务行为。
