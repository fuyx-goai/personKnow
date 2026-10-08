# PersonKnow 个人知识库

PersonKnow 是一个按 Figma「个人知识库小程序」原型实现的全栈知识库应用，包含微信原生小程序、Vue Web、Go API、异步索引 Worker、PostgreSQL 持久化与 Mem/Milvus 向量检索。

## 功能

- 微信登录与 Web 扫码登录，访问令牌刷新、设备会话与安全退出；
- 私有/公开多知识库，公开库只读访问与独立 RAG 参数；
- PDF、DOCX、Markdown、TXT、HTML、CSV、PPTX 上传、编辑、重命名与重新索引；
- 多知识库流式问答、历史会话、引用溯源与 Token 信息；
- 存储/Token 用量、审计日志、账号安全与 JSON 备份；
- 旧 `knowledge.json` 幂等迁移到指定用户的默认知识库。

## 技术栈

- 后端：Go 1.24、Gin、pgx、Eino；
- 数据：PostgreSQL、本地文件存储、Mem 或 Milvus；
- Web：Vue 3、Vite、Vitest；
- 小程序：微信原生小程序。

## 本地启动

1. 准备 PostgreSQL 数据库，并复制示例配置：

   ```bash
   cp configs/config.example.yaml configs/config.local.yaml
   ```

2. 通过环境变量注入敏感配置：

   ```bash
   export CONFIG_FILE=configs/config.local.yaml
   export DATABASE_URL='postgres://user:password@127.0.0.1:5432/personknow?sslmode=disable'
   export JWT_SECRET='至少32字符的随机值'
   export IDENTITY_SECRET='至少32字符的随机值'
   export WECHAT_APP_ID='wx...'
   export WECHAT_APP_SECRET='...'
   export LLM_API_KEY='...'
   ```

3. 初始化数据库并启动服务：

   ```bash
   go run ./cmd/migrate up
   go run ./cmd/gateway
   ```

4. 浏览器打开 `http://127.0.0.1:8080`。健康检查为 `GET /api/health`，完整业务 API 位于 `/api/v1`。

## 前端开发

```bash
cd web
npm ci
npm test -- --run
npm run build
```

Web 构建产物输出到 `internal/gateway/web/dist` 并嵌入 Go 服务。小程序导入与发布说明见 [miniprogram/README.md](miniprogram/README.md)。

## 旧数据迁移

先确认目标用户 UUID，再执行：

```bash
go run ./cmd/migrate legacy \
  --target-user '<user-uuid>' \
  --source './knowledge.json' \
  --backup-dir './backups'
```

命令会先备份源文件，再创建“默认知识库”，按来源与内容哈希幂等导入。迁移报告只包含来源名和错误码，不输出正文。详细部署、恢复与权限要求见 [docs/deploy-notes.md](docs/deploy-notes.md)。

## 验证

```bash
go test ./...
cd web && npm test -- --run && npm run build
cd .. && node --test miniprogram/tests/*.test.js
find miniprogram -name '*.js' -type f -print0 | xargs -0 -n1 node --check
```
