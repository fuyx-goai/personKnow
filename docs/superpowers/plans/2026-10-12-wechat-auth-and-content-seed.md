# 微信授权登录与教学内容导入 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 将小程序登录改成用户主动触发的微信授权登录，并向当前用户的真实知识库幂等导入 10 篇 AI 开发文章及许可明确的 Go 教学资料。

**Architecture:** 登录仍复用现有 Go `code2Session` 接口，但会话恢复和首次授权分离：已有 refresh token 静默刷新，没有会话则停留在登录页。内容导入复用 `internal/demo` 的分层与 PostgreSQL 仓储，由静态 AI 文章目录和受控 GitHub 来源清单共同生成文档，所有外部内容保留来源与许可证。

**Tech Stack:** 微信原生小程序、Node.js `node:test`、Go、PostgreSQL、GitHub Raw HTTPS。

## Global Constraints

- AppSecret 只能存在于 Go 私密配置，不进入小程序源码、日志或文档。
- `wx.login` 只能在用户点击授权按钮后调用；已有 refresh token 可静默恢复。
- GitHub 来源仅允许 `github.com` / `raw.githubusercontent.com` 且必须记录许可证。
- 数据命令必须幂等，不删除或覆盖用户自建知识库与文档。
- 新增 Go 代码遵守 `internal/<domain>/{entity,repo,service}` 目录约束。

---

### Task 1: 登录状态机

**Files:**
- Modify: `miniprogram/services/auth.js`
- Modify: `miniprogram/app.js`
- Test: `miniprogram/tests/auth.test.js`

**Interfaces:**
- Produces: `auth.hasSession(): boolean`、`auth.restore(): Promise<AuthResult>`、`app.authorize(profile): Promise<GlobalData>`。

- [x] 新增失败测试，证明无 refresh token 时 `restore()` 返回 `AUTH_REQUIRED` 且不调用 `wx.login`。
- [x] 运行 `node --test miniprogram/tests/auth.test.js` 确认测试因旧的自动登录行为失败。
- [x] 将静默恢复限制为 refresh token，并在 `app.authorize()` 中显式执行登录交换。
- [x] 重跑定向测试并确认通过。

### Task 2: 微信授权登录页

**Files:**
- Create: `miniprogram/pages/login/index.{js,json,wxml,wxss}`
- Modify: `miniprogram/app.json`
- Modify: `miniprogram/pages/profile/index.js`
- Modify: `miniprogram/README.md`

**Interfaces:**
- Consumes: `app.authorize({ nickname, device_label })`、`app.restoreSession()`。
- Produces: 首次授权页、会话恢复、拒绝/失败后的重试与退出登录回跳。

- [x] 登录页先尝试恢复已有会话；无会话时展示昵称输入、头像选择和授权按钮。
- [x] 授权按钮调用 `app.authorize`，成功后 `wx.reLaunch('/pages/chat/index')`。
- [x] 退出登录清除会话并 `reLaunch` 登录页。
- [x] 更新小程序登录与联调说明。

### Task 3: AI 与 Go 教学内容目录

**Files:**
- Create: `internal/demo/entity/ai_articles.go`
- Create: `internal/demo/entity/go_tutorials.go`
- Modify: `internal/demo/entity/model.go`
- Modify: `internal/demo/entity/model_test.go`

**Interfaces:**
- Produces: `AIArticles() []DocumentSeed`、`GoTutorialSources() []RemoteDocumentSeed`。

- [x] 新增测试，要求 AI 文章恰好 10 篇且每篇包含正文、摘要和标签。
- [x] 新增测试，要求远程来源为 HTTPS GitHub 地址并声明许可证。
- [x] 编写覆盖 LLM API、Prompt、Embedding、RAG、Agent、工具调用、评测、可观测性、安全和部署的 10 篇原创文章。
- [x] 建立 Go 官方教程白名单，包含仓库、原文 URL、许可证和目标文档名。

### Task 4: 受控抓取与幂等落库

**Files:**
- Create: `internal/demo/service/content_fetcher.go`
- Modify: `internal/demo/service/service.go`
- Modify: `internal/demo/repo/postgres_repository.go`
- Modify: `cmd/demo-seed/main.go`
- Test: `internal/demo/service/content_fetcher_test.go`
- Test: `internal/demo/repo/postgres_repository_test.go`

**Interfaces:**
- Produces: 只允许受控 GitHub 主机、限制响应大小与超时的抓取器；`demo-seed -content` 导入入口。

- [x] 先测试拒绝非 HTTPS/非 GitHub 地址、非 2xx 响应和超限正文。
- [x] 实现超时、状态码、大小限制和来源头部拼接。
- [x] 将原创与远程资料合并到稳定 ID 数据集，保留现有资料并更新统计。
- [x] 重跑 Go 定向测试并确认幂等数量。

### Task 5: 实际写入与交付门禁

**Files:**
- Modify: `README.md`
- Modify: `miniprogram/README.md`

- [x] 运行小程序全量测试、JS 语法检查和 `git diff --check`。
- [x] 运行 `gofmt -w`、`go test ./...` 和 `go vet ./...`。
- [x] 运行安全扫描，Critical/High 必须为 0。
- [x] 对当前 PostgreSQL 最近登录用户执行内容导入，并查询知识库与文档数量。
- [x] 通过 `/api/v1/status` 和小程序登录接口日志做冒烟核对，不输出微信 code、AppSecret 或 token。

> 微信开发者工具 CLI 编译验证仍需在开发者工具的“安全设置”中开启服务端口后执行；当前网关接口和自动化测试已完成验证。
