# 个人知识库全栈版本 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 将现有单用户知识库升级为严格对应 Figma 原型的微信小程序、Web 与 Go 全栈多用户版本。

**Architecture:** 使用 Go 模块化单体承载 Gin API 与 PostgreSQL 持久化 Worker，本地文件目录保存原文件和文本版本，Mem/Milvus 保存带访问元数据的向量。所有客户端能力通过 `/api/v1` 真实 API 驱动，旧 `/api` 在迁移期保留兼容。

**Tech Stack:** Go 1.24、Gin、pgx/v5、PostgreSQL、JWT、Eino、Mem/Milvus、Vue/Vite、微信原生小程序。

---

## 文件结构

新增模块及职责：

```text
cmd/gateway/                  组合启动 API、数据库与 Worker
cmd/migrate/                  schema 和旧 knowledge.json 迁移命令
internal/account/             微信、JWT、刷新会话、扫码登录
internal/library/             知识库、访问策略、RAG 参数
internal/document/            文件元数据、内容版本、解析器、FileStore
internal/indexing/            PostgreSQL 任务状态机与 Worker
internal/chat/                会话、消息、检索范围与引用
internal/usage/               配额、流水、汇总与审计
internal/platform/httpx/      request_id、错误体、鉴权中间件
internal/platform/logging/    JSON 日志与滚动输出
internal/platform/postgres/   pgx、事务和 SQL migration
internal/platform/vector/     Mem/Milvus 统一范围检索端口
web/src/                      Web 登录与原型功能页面
miniprogram/                  微信登录与原型四页真实 API
```

### Task 1：安全配置、错误体系与结构化日志

**Files:**
- Modify: `.gitignore`
- Modify: `pkg/config/config.go`
- Create: `pkg/config/config_test.go`
- Create: `configs/config.example.yaml`
- Create: `internal/platform/httpx/error.go`
- Create: `internal/platform/httpx/request_id.go`
- Create: `internal/platform/httpx/request_id_test.go`
- Create: `internal/platform/logging/logger.go`
- Create: `internal/platform/logging/logger_test.go`
- Modify: `go.mod`

- [x] **Step 1: 写失败测试，覆盖环境变量覆盖、request_id 和敏感字段过滤**

```go
func TestLoadUsesEnvironmentSecrets(t *testing.T) {
    t.Setenv("DATABASE_URL", "postgres://user:pass@localhost/knowledge")
    t.Setenv("JWT_SECRET", "01234567890123456789012345678901")
    cfg, err := Load()
    if err != nil { t.Fatal(err) }
    if cfg.Database.URL == "" || cfg.Auth.JWTSecret == "" { t.Fatal("missing secrets") }
}
```

- [x] **Step 2: 运行定向测试并确认失败**

Run: `go test ./pkg/config ./internal/platform/httpx ./internal/platform/logging`

Expected: FAIL，提示新类型或函数未定义。

- [x] **Step 3: 实现配置和基础设施**

```go
type DatabaseConfig struct { URL string; MaxConns int32 }
type AuthConfig struct {
    JWTSecret string
    AccessTTL time.Duration
    RefreshTTL time.Duration
    WeChatAppID string
    WeChatSecret string
}
type StorageConfig struct { DataDir string; MaxFileBytes int64 }
type WorkerConfig struct { Concurrency int; Lease time.Duration; Poll time.Duration }
type LogConfig struct { Dir string; MaxSizeMB int; RetainDays int }
```

错误响应固定为 `code/message/request_id/details`；日志使用 `slog.JSONHandler`，通过白名单属性记录 request、worker 和模型元数据。

- [x] **Step 4: 更新忽略规则并提供无密钥示例配置**

`.gitignore` 必须忽略 `configs/config.yaml`、`configs/config.local.yaml` 和 `miniprogram/project.private.config.json`；示例文件中的所有敏感值为空或环境变量说明。

- [x] **Step 5: 运行测试与格式化**

Run: `gofmt -w pkg/config internal/platform && go test ./pkg/config ./internal/platform/...`

Expected: PASS。

- [x] **Step 6: 提交**

```bash
git add .gitignore configs/config.example.yaml go.mod go.sum pkg/config internal/platform
git commit -m "feat(infra): 增加安全配置与结构化日志"
```

### Task 2：PostgreSQL schema、连接与事务基座

**Files:**
- Create: `internal/platform/postgres/db.go`
- Create: `internal/platform/postgres/tx.go`
- Create: `internal/platform/postgres/migrate.go`
- Create: `internal/platform/postgres/migrations/001_init.sql`
- Create: `internal/platform/postgres/migrate_test.go`
- Create: `cmd/migrate/main.go`

- [x] **Step 1: 写 migration 失败测试**

```go
func TestMigrationsContainRequiredTables(t *testing.T) {
    sql := migrationsForTest(t)
    for _, table := range []string{"users", "knowledge_bases", "documents", "index_jobs", "usage_records", "audit_logs"} {
        if !strings.Contains(sql, "CREATE TABLE "+table) { t.Fatalf("missing %s", table) }
    }
}
```

- [x] **Step 2: 运行测试确认失败**

Run: `go test ./internal/platform/postgres`

Expected: FAIL，migration 尚不存在。

- [x] **Step 3: 实现完整首版 migration**

SQL 必须创建设计文档中的 18 张业务表、`wechat_identities`、`schema_migrations`、CHECK 约束、外键、联合索引和部分唯一索引；启用 `pg_trgm` 扩展。

- [x] **Step 4: 实现 pgxpool、事务接口和显式 migration 命令**

```go
type DBTX interface {
    Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
    Query(context.Context, string, ...any) (pgx.Rows, error)
    QueryRow(context.Context, string, ...any) pgx.Row
}

func WithinTx(ctx context.Context, pool *pgxpool.Pool, fn func(pgx.Tx) error) error
```

- [x] **Step 5: 验证**

Run: `go test ./internal/platform/postgres ./cmd/migrate`

Expected: PASS；若 `TEST_DATABASE_URL` 存在，额外实际执行 up/status 并回滚临时 schema。

- [x] **Step 6: 提交**

```bash
git add cmd/migrate internal/platform/postgres go.mod go.sum
git commit -m "feat(db): 建立PostgreSQL数据基座"
```

### Task 3：账号、JWT、刷新会话与 Web 扫码登录

**Files:**
- Create: `internal/account/model.go`
- Create: `internal/account/ports.go`
- Create: `internal/account/service.go`
- Create: `internal/account/token.go`
- Create: `internal/account/wechat.go`
- Create: `internal/account/postgres_repository.go`
- Create: `internal/account/service_test.go`
- Create: `internal/account/handler.go`
- Create: `internal/platform/httpx/auth.go`

- [x] **Step 1: 写账号状态机和票据消费失败测试**

```go
func TestConfirmTicketOnlyOnce(t *testing.T) {
    service := newAccountServiceForTest(t)
    ticket := service.CreateWebTicket(context.Background())
    if err := service.ConfirmWebTicket(ctx, userID, ticket.Secret); err != nil { t.Fatal(err) }
    if err := service.ConfirmWebTicket(ctx, userID, ticket.Secret); !errors.Is(err, ErrTicketInvalid) { t.Fatalf("got %v", err) }
}
```

- [x] **Step 2: 实现微信客户端与令牌服务**

`WeChatClient.ExchangeCode` 调用官方 `jscode2session`；JWT claims 只包含 `sub/session_id/exp/iat`；刷新令牌使用 32 字节随机值并只保存 SHA-256 哈希。

- [x] **Step 3: 实现账号 repository 和 service**

事务内完成微信身份 upsert、默认套餐分配、会话创建、票据条件确认和票据消费。OpenID 使用 HMAC 查找值与 AES-GCM 密文存储。

- [x] **Step 4: 实现 handler 与鉴权中间件**

覆盖 `/auth/wechat/login`、`/auth/refresh`、`/auth/logout`、`/auth/web/tickets`、票据查询与确认、`/me` 和会话撤销接口。

- [x] **Step 5: 运行测试**

Run: `go test ./internal/account ./internal/platform/httpx`

Expected: PASS，包含过期票据、重复消费、撤销会话和非法 JWT。

- [x] **Step 6: 提交**

```bash
git add internal/account internal/platform/httpx
git commit -m "feat(auth): 实现微信与扫码登录"
```

### Task 4：多知识库、公开访问与 RAG 参数

**Files:**
- Create: `internal/library/model.go`
- Create: `internal/library/policy.go`
- Create: `internal/library/repository.go`
- Create: `internal/library/service.go`
- Create: `internal/library/handler.go`
- Create: `internal/library/service_test.go`

- [x] **Step 1: 写权限矩阵测试**

```go
func TestAccessPolicy(t *testing.T) {
    tests := []struct{ owner, actor uuid.UUID; visibility string; want Access }{
        {ownerID, ownerID, "private", AccessOwner},
        {ownerID, otherID, "public", AccessRead},
        {ownerID, otherID, "private", AccessNone},
    }
    for _, tc := range tests { /* assert DecideAccess */ }
}
```

- [x] **Step 2: 实现 CRUD、列表与统计**

列表返回 `owned/public` 分组；公开列表排除当前用户自己的库；删除只标记 `deleting` 并创建清理任务。

- [x] **Step 3: 实现 RAG 参数校验**

```go
func (s RetrievalSettings) Validate() error {
    if s.ChunkSize < 200 || s.ChunkSize > 2000 { return ErrChunkSize }
    if s.ChunkOverlap < 0 || s.ChunkOverlap > 500 || s.ChunkOverlap >= s.ChunkSize { return ErrChunkOverlap }
    if s.TopK < 1 || s.TopK > 20 { return ErrTopK }
    if s.SimilarityThreshold < 0 || s.SimilarityThreshold > 1 { return ErrThreshold }
    return nil
}
```

- [x] **Step 4: 实现 API 和审计挂钩**

覆盖知识库 CRUD、公开列表、统计、检索设置和库级重新索引；所有写操作调用审计端口。

- [x] **Step 5: 验证并提交**

Run: `go test ./internal/library`

```bash
git add internal/library
git commit -m "feat(library): 支持多知识库与检索设置"
```

### Task 5：本地文件存储、七类解析器与内容版本

**Files:**
- Create: `internal/document/model.go`
- Create: `internal/document/filestore.go`
- Create: `internal/document/validate.go`
- Create: `internal/document/parser.go`
- Create: `internal/document/parser_text.go`
- Create: `internal/document/parser_office.go`
- Create: `internal/document/parser_pdf.go`
- Create: `internal/document/repository.go`
- Create: `internal/document/service.go`
- Create: `internal/document/handler.go`
- Create: `internal/document/testdata/*`
- Create: `internal/document/service_test.go`

- [x] **Step 1: 写路径、格式和配额失败测试**

```go
func TestFileStoreNeverUsesClientNameInPath(t *testing.T) {
    store := NewLocalFileStore(t.TempDir())
    path, err := store.SaveOriginal(ctx, ids, "../../secret.txt", strings.NewReader("safe"))
    if err != nil { t.Fatal(err) }
    if strings.Contains(path, "..") || strings.Contains(path, "secret.txt") { t.Fatalf("unsafe path %s", path) }
}
```

- [x] **Step 2: 实现原子 FileStore**

临时文件与正式目录在同一文件系统；计算 SHA-256；成功事务后 rename；失败时删除临时文件；所有路径使用 UUID。

- [x] **Step 3: 实现解析器注册表**

`txt/md/csv/html` 使用标准库与 `x/net/html`；DOCX/PPTX 读取 ZIP XML；PDF 使用明确依赖提取文本和页码映射。输出统一 `ParsedContent{Text, StructureMap}`。

- [x] **Step 4: 实现文档 service 与 API**

上传校验 50 MiB、权限和剩余容量后创建文档与索引任务；编辑生成新版本；重命名只修改展示名；删除创建清理任务。

- [x] **Step 5: 七类样本验证**

Run: `go test ./internal/document -run 'TestParse|TestUpload|TestEdit|TestDelete'`

Expected: 七种格式均得到非空 UTF-8 文本；伪造格式、超限和路径穿越被拒绝。

- [x] **Step 6: 提交**

```bash
git add internal/document go.mod go.sum
git commit -m "feat(document): 实现文件全生命周期"
```

### Task 6：用量、配额和审计事务

**Files:**
- Create: `internal/usage/model.go`
- Create: `internal/usage/repository.go`
- Create: `internal/usage/service.go`
- Create: `internal/usage/handler.go`
- Create: `internal/usage/service_test.go`

- [ ] **Step 1: 写并发配额与月份边界测试**

测试 UTC 月首日、存储释放、Embedding/Input/Output 汇总，以及两个并发预留不能共同超过额度。

- [ ] **Step 2: 实现用量预留和结算接口**

```go
type QuotaService interface {
    ReserveTokens(ctx context.Context, userID uuid.UUID, estimated int64) (Reservation, error)
    SettleTokens(ctx context.Context, reservation Reservation, actual Breakdown) error
    ApplyStorage(ctx context.Context, userID, resourceID uuid.UUID, delta int64) error
    Summary(ctx context.Context, userID uuid.UUID, now time.Time) (Summary, error)
}
```

- [ ] **Step 3: 实现审计写入和查询**

审计 metadata 只允许资源名称、状态、错误码和参数差异，不接受自由正文；查询强制 `actor_user_id=current_user`。

- [ ] **Step 4: 验证并提交**

Run: `go test ./internal/usage`

```bash
git add internal/usage
git commit -m "feat(usage): 增加配额统计与审计"
```

### Task 7：版本化向量端口与异步索引 Worker

**Files:**
- Create: `internal/platform/vector/model.go`
- Create: `internal/platform/vector/repository.go`
- Create: `internal/platform/vector/contract_test.go`
- Modify: `internal/knowledge/repo/vectorstore/adapter.go`
- Modify: `internal/knowledge/repo/vectorstore/mem.go`
- Modify: `internal/knowledge/repo/vectorstore/milvus.go`
- Create: `internal/indexing/model.go`
- Create: `internal/indexing/repository.go`
- Create: `internal/indexing/worker.go`
- Create: `internal/indexing/worker_test.go`

- [ ] **Step 1: 写 Mem/Milvus 共同契约测试**

```go
type SearchScope struct {
    LibraryIDs []uuid.UUID
    ActiveVersions map[uuid.UUID]uuid.UUID
    MinSimilarity float64
}
```

契约测试必须验证：私有范围过滤、内容版本过滤、按版本删除、失败不切换和稳定 chunk ID。

- [ ] **Step 2: 改造向量元数据**

统一字段为 `owner_user_id/library_id/document_id/content_version_id/chunk_index/source_name/page_or_section/visibility`；禁止继续按文件名作为删除边界。

- [ ] **Step 3: 实现任务领取、租约和恢复**

领取 SQL 使用 `FOR UPDATE SKIP LOCKED`；Worker 默认并发 2；启动时将租约过期的 running 任务重新排队。

- [ ] **Step 4: 实现索引阶段**

解析或读取编辑文本 → 按知识库设置切块 → 预留 Token → Embedding → 写新版本向量 → 事务切换 active version → 删除旧向量 → 结算用量和审计。

- [ ] **Step 5: 验证并提交**

Run: `go test ./internal/platform/vector ./internal/indexing ./internal/knowledge/repo/vectorstore`

```bash
git add internal/platform/vector internal/indexing internal/knowledge/repo/vectorstore
git commit -m "feat(indexing): 实现版本化异步索引"
```

### Task 8：会话、范围检索、SSE 与引用持久化

**Files:**
- Create: `internal/chat/model.go`
- Create: `internal/chat/repository.go`
- Create: `internal/chat/service.go`
- Create: `internal/chat/handler.go`
- Create: `internal/chat/service_test.go`
- Modify: `internal/knowledge/repo/chat_pipeline.go`

- [ ] **Step 1: 写范围与中断测试**

覆盖单库、全域、公开库、他人私有库排除、Token 超额前置拒绝和客户端取消后 `interrupted` 状态。

- [ ] **Step 2: 实现会话与范围计算**

全域范围只包含自己的 active 库和他人的 public active 库；每次问答重新计算，避免可见性变化后继续访问。

- [ ] **Step 3: 实现 SSE 协议**

固定事件类型 `delta/reference/usage/error/done`；所有错误事件包含稳定 code 和 request_id；完成时保存消息、引用快照和实际 Token。

- [ ] **Step 4: 实现历史查询与删除**

历史记录按 `updated_at,id` 游标分页；删除仅允许会话所有者并写审计。

- [ ] **Step 5: 验证并提交**

Run: `go test ./internal/chat ./internal/knowledge/repo`

```bash
git add internal/chat internal/knowledge/repo/chat_pipeline.go
git commit -m "feat(chat): 支持多库问答与引用历史"
```

### Task 9：组合路由、启动流程与旧 API 兼容

**Files:**
- Modify: `cmd/gateway/main.go`
- Modify: `cmd/gateway/main_test.go`
- Modify: `internal/gateway/router/router.go`
- Modify: `internal/gateway/router/v1.go`
- Create: `internal/gateway/router/v1_test.go`
- Create: `internal/gateway/handler/compat.go`

- [ ] **Step 1: 写 API 路由与错误契约测试**

验证公开路由、受保护路由、request_id 响应头、统一错误体、CORS 和 SSE 头。

- [ ] **Step 2: 组合所有模块**

启动顺序：配置 → logger → PostgreSQL → schema 版本检查 → 模型与向量库 → services → Worker → HTTP；关闭时先停止接收请求，再取消 Worker，最后关闭连接池。

- [ ] **Step 3: 保留兼容 API**

旧 `/api/health/config/stats/chunks/ingest/chat` 在迁移期继续可用；日志标记 `legacy_api=true`，但不允许绕过新鉴权访问多用户数据。

- [ ] **Step 4: 验证并提交**

Run: `go test ./cmd/gateway ./internal/gateway/...`

```bash
git add cmd/gateway internal/gateway
git commit -m "feat(api): 接入全栈业务路由"
```

### Task 10：微信小程序严格接入原型功能

**Files:**
- Modify: `miniprogram/app.js`
- Modify: `miniprogram/services/api.js`
- Create: `miniprogram/services/auth.js`
- Create: `miniprogram/services/upload.js`
- Modify: `miniprogram/pages/chat/*`
- Modify: `miniprogram/pages/libraries/*`
- Modify: `miniprogram/pages/sources/*`
- Modify: `miniprogram/pages/profile/*`
- Create: `miniprogram/components/library-form/*`
- Create: `miniprogram/components/document-editor/*`
- Create: `miniprogram/tests/auth.test.js`
- Create: `miniprogram/tests/api.test.js`

- [ ] **Step 1: 写客户端状态与 API 测试**

覆盖令牌刷新、401 单次重试、公开库只读、任务轮询、配额错误和 SSE 五类事件。

- [ ] **Step 2: 实现微信登录与会话**

使用 `wx.login` 获取 code；访问令牌仅保存在内存，刷新令牌使用小程序安全存储；退出时清理所有本地状态。

- [ ] **Step 3: 接入四个原型页面**

知识库页接入我的/公开混排；资料页接入 `wx.chooseMessageFile` 上传、编辑、重命名、索引和删除；问答页接入历史与来源；我的空间接入三个页签。

- [ ] **Step 4: 保持原型视觉**

不新增底部入口；复用顶部栏、卡片、标签、按钮和弹层语言；公开卡片仅增加所有者和只读小标签。

- [ ] **Step 5: 验证并提交**

Run: `node --test miniprogram/tests/*.test.js && find miniprogram -name '*.js' -type f -print0 | xargs -0 -n1 node --check`

```bash
git add miniprogram
git commit -m "feat(miniprogram): 接入完整知识库能力"
```

### Task 11：Web 扫码登录与完整业务页面

**Files:**
- Modify: `web/package.json`
- Modify: `web/src/api.js`
- Modify: `web/src/store.js`
- Modify: `web/src/App.vue`
- Create: `web/src/views/LoginView.vue`
- Create: `web/src/views/LibrariesView.vue`
- Create: `web/src/views/DocumentsView.vue`
- Create: `web/src/views/ProfileView.vue`
- Modify: `web/src/views/AskView.vue`
- Modify: `web/src/styles/base.css`
- Modify: `web/src/styles/library.css`
- Create: `web/src/utils/auth.js`
- Create: `web/src/utils/auth.test.js`

- [ ] **Step 1: 写扫码轮询与刷新测试**

使用 Vitest 验证票据 `pending/confirmed/expired`、访问令牌刷新和 401 重试只发生一次。

- [ ] **Step 2: 实现扫码登录**

创建票据后生成本地二维码；2 秒轮询，确认后保存刷新会话；组件卸载时停止轮询并废弃过期票据。

- [ ] **Step 3: 接入知识库、资料、问答和空间页面**

页面信息结构与小程序保持一致；文件上传使用浏览器 multipart；编辑文本使用弹层；任务状态轮询到终态停止。

- [ ] **Step 4: 构建并提交**

Run: `npm test -- --run && npm run build`

```bash
git add web internal/gateway/web/dist
git commit -m "feat(web): 实现扫码登录与多库管理"
```

### Task 12：旧数据迁移与运行文档

**Files:**
- Create: `internal/migration/legacy.go`
- Create: `internal/migration/legacy_test.go`
- Modify: `cmd/migrate/main.go`
- Modify: `docs/deploy-notes.md`
- Modify: `miniprogram/README.md`
- Create: `README.md`

- [ ] **Step 1: 写幂等迁移测试**

同一 `knowledge.json` 连续迁移两次，第二次新增文档数和片段数必须为 0；失败报告包含来源名与错误码但不含正文。

- [ ] **Step 2: 实现迁移命令**

命令要求 `--target-user`、`--source` 和 `--backup-dir`；先备份，创建“默认知识库”，按来源和内容哈希导入文档与向量元数据。

- [ ] **Step 3: 更新部署文档**

记录 PostgreSQL 初始化、环境变量、目录权限、migration、启动、健康检查、微信域名配置、日志轮转和恢复步骤。

- [ ] **Step 4: 验证并提交**

Run: `go test ./internal/migration ./cmd/migrate`

```bash
git add README.md docs miniprogram/README.md internal/migration cmd/migrate
git commit -m "feat(migration): 增加旧数据迁移工具"
```

### Task 13：全链路验证、安全门禁与合并

**Files:**
- Create: `internal/integration/fullstack_test.go`
- Create: `docs/verification/fullstack-knowledge-base.md`

- [ ] **Step 1: 运行双用户集成场景**

用户 A 私有库对 B 返回 404；A 公开后 B 可问答但所有写操作返回 403；容量与 Token 超限在上传、索引和模型调用前被拒绝。

- [ ] **Step 2: 运行完整验证**

```bash
go test ./...
cd web && npm test -- --run && npm run build
cd .. && node --test miniprogram/tests/*.test.js
find miniprogram -name '*.js' -type f -print0 | xargs -0 -n1 node --check
```

Expected: 全部退出码为 0。

- [ ] **Step 3: 执行安全检查**

确认 Git 暂存区不包含 `configs/config.yaml`、微信私有配置、API Key、JWT 密钥、数据库密码或真实 OpenID；验证日志测试不会输出正文或令牌。

- [ ] **Step 4: 记录验证证据并提交**

```bash
git add internal/integration docs/verification internal/gateway/web/dist
git commit -m "test(fullstack): 完成全链路验收"
git push origin feat/fullstack-knowledge-base
```

- [ ] **Step 5: 审查并合并**

仅在分支工作区干净、远端已同步、全部验证通过且没有高优先级审查问题时，将 `feat/fullstack-knowledge-base` 合并到 `main` 并推送；合并后在 `main` 再执行一次 `go test ./...` 和 Web 构建。
