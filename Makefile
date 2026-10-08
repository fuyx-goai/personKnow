# Makefile —— 常用命令入口（与 bw-cli 脚手架的根目录约定保持一致）
#
# 用法：make <目标>；直接执行 make 会列出全部可用命令。

APP     := knowledge-base
ENTRY   := ./cmd/gateway
BIN_DIR := bin

.DEFAULT_GOAL := help

.PHONY: help
help: ## 显示所有可用命令
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-10s\033[0m %s\n", $$1, $$2}'

.PHONY: tidy
tidy: ## 整理依赖（go mod tidy）
	go mod tidy

.PHONY: fmt
fmt: ## 格式化代码（gofmt）
	gofmt -w .

.PHONY: vet
vet: ## 静态检查（go vet）
	go vet ./...

.PHONY: test
test: ## 跑全部测试（不需要 API Key，也不需要 Milvus）
	go test ./... -count=1

.PHONY: check
check: fmt vet test ## 格式化 + 静态检查 + 测试

.PHONY: build
build: ## 编译二进制到 bin/
	go build -o $(BIN_DIR)/$(APP) $(ENTRY)

.PHONY: run
run: ## 本地启动网关进程（读取 configs/config.yaml，同时提供前端页面）
	go run $(ENTRY)

# ---------- 前端：Vue 3 + Vite（源码在 web/，产物嵌进 Go 二进制）----------
# 日常改 Go 代码不需要 Node：dist/ 已经提交在仓库里，go build 直接可用。
# 只有动了 web/ 里的前端源码，才需要跑一次 make web。

.PHONY: web
web: ## 构建前端 -> internal/gateway/web/dist（改了前端才需要）
	cd web && npm install --no-audit --no-fund && npm run build

.PHONY: web-deps
web-deps: ## 只安装前端依赖（首次开发时用）
	cd web && npm install --no-audit --no-fund

.PHONY: dev-web
dev-web: ## 前端热更新开发模式（另开一个终端跑 make run 提供接口）
	cd web && npm run dev

.PHONY: clean
clean: ## 清理编译产物
	rm -rf $(BIN_DIR)
