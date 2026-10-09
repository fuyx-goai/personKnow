# 全栈知识库验收证据

日期：2026-10-08

分支：`feat/fullstack-knowledge-base`

## 验收范围

- Go API、PostgreSQL repository、索引 Worker、Mem/Milvus 适配层；
- Vue Web 扫码登录与四个原型入口；
- 微信原生小程序登录、知识库、原资料、问答、我的空间；
- 旧 `knowledge.json` 备份、幂等迁移、错误脱敏；
- 结构化日志、用量统计与审计日志。

## 自动化验证

### Go 与静态检查

```bash
go test ./...
go vet ./...
```

结果：退出码均为 0。`cmd/gateway`、账号、知识库、文件、索引、问答、用量、迁移、平台基础设施等包通过。

### Web

```bash
cd web
npm test -- --run
npm run build
```

结果：5 项 Vitest 通过；Vite 8.3.3 完成 68 个模块构建；产物同步到 `internal/gateway/web/dist`。

### 微信小程序

```bash
node --test miniprogram/tests/*.test.js
find miniprogram -name '*.js' -type f -print0 | xargs -0 -n1 node --check
```

结果：19 项测试通过，全部 JavaScript 文件语法检查通过。

## 真实 PostgreSQL 集成

使用本机 Docker `postgres:16-alpine` 创建仅绑定 `127.0.0.1` 随机端口的临时数据库；测试结束后容器自动停止并删除。

```bash
TEST_DATABASE_URL='postgres://.../personknow_test?sslmode=disable' \
  go test -count=1 -v \
  ./internal/platform/postgres ./internal/account/... ./internal/library/... \
  ./internal/document/... ./internal/indexing/... ./internal/chat/... ./internal/usage/... \
  ./internal/migration/... ./internal/integration
```

结果：全部通过，包括：

- 用户 A 的私有库对用户 B 隐藏；
- A 公开知识库后，B 可创建范围化会话并完成问答；
- B 修改公开库或向公开库上传文件被拒绝；
- 存储配额为 0 时，上传不创建文档、不调度索引任务；
- Token 配额为 0 时，索引不调用 Embedder、不写向量；
- Token 配额为 0 时，问答不调用模型、不创建消息；
- 旧数据重复迁移时，第二次新增文档数和片段数均为 0。

## 安全与质量门禁

- `verify-change`：通过；README、部署文档、PRD、数据库设计、架构图、技术方案和实施计划已同步；
- `verify-quality`：通过，错误 0、警告 0；仅保留非阻塞行长提示；
- `verify-security`：`internal`、`cmd`、`web/src`、`miniprogram` 均通过，Critical/High/Medium/Low 全为 0；
- `configs/config.yaml`、`miniprogram/project.private.config.json`、`knowledge.json` 未被 Git 跟踪且命中忽略规则；
- 跟踪文件密钥模式扫描无命中；
- 日志、用量审计与迁移失败报告测试确认不输出正文、令牌或身份密钥；
- 分支变更文件规模检查无超过 300 行的代码文件。

## 界面烟测

- Web 1280px 登录页保持原型双栏结构，无横向溢出；
- 页面使用深海军蓝容器、白色主体、青绿色主色、白卡片与柔和阴影；
- 问答、知识库、原资料、我的空间四个入口与 Figma 原型一致；
- 浏览器控制台无前端运行时错误。

## 结论

自动化、真实 PostgreSQL 集成、安全与质量门禁均通过，无已知 Critical/High 问题。满足推送功能分支并执行合并前审查的条件。
