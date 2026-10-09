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

## 后端架构

后端目录参考 [BwCloudWeGo/bw-cli](https://github.com/BwCloudWeGo/bw-cli) 的模块化单体约定：`internal/gateway` 统一承载 HTTP 协议入口，各业务域按 `entity/handler/repo/service` 分层，`internal/platform` 只放公共基础设施。

```text
internal/
  gateway/                 handler / request / router / web
  account/                 entity / handler / repo / service
  library/                 entity / handler / repo / service
  document/                entity / handler / repo / service
  indexing/                entity / handler / repo / service
  chat/                    entity / handler / repo / service
  usage/                   entity / handler / repo / service
  migration/               entity / repo / service
  platform/                httpx / logging / postgres / vector
```

详细依赖方向见 [docs/architecture/backend-layout.md](docs/architecture/backend-layout.md)，强制开发约束见 [AGENTS.md](AGENTS.md)。

## 项目环境

| 依赖 | 版本/要求 | 用途 |
| --- | --- | --- |
| Go | 1.24.1 或兼容的 1.24.x | 编译、运行 API 与索引 Worker |
| PostgreSQL | 15+，可创建 `pg_trgm` 扩展 | 账号、知识库、文档、问答、用量和审计数据 |
| Node.js | 20+ | Web 开发、测试和重新构建；只运行已构建二进制时不需要 |
| npm | 使用仓库内 `package-lock.json` | 安装锁定的 Web 依赖 |
| Milvus | 2.4，可选 | 生产级向量检索；本地默认使用 `mem` |
| 微信开发者工具 | 当前稳定版 | 小程序预览、真机调试和发布 |

推荐在 macOS/Linux 开发。Windows 可使用 WSL2，并确保 Go、Node.js 和 PostgreSQL 均可从终端访问。

## 本地启动

### 1. 获取代码并检查依赖

```bash
git clone https://github.com/fuyx-goai/personKnow.git
cd personKnow
go version
node --version
psql --version
```

### 2. 准备 PostgreSQL

可复用已有 PostgreSQL 并创建数据库，随后把 `DATABASE_URL` 中的账号、密码改为实际值：

```bash
createdb personknow
```

也可以用 Docker 启动本地 PostgreSQL 16：

```bash
docker run --name personknow-postgres \
  -e POSTGRES_USER=personknow \
  -e POSTGRES_PASSWORD=personknow \
  -e POSTGRES_DB=personknow \
  -p 5432:5432 \
  -d postgres:16
```

首次 migration 会创建 `pg_trgm` 扩展，因此数据库账号需要相应权限。

### 3. 准备私有配置

复制示例配置。`configs/config.local.yaml` 已被 Git 忽略：

```bash
cp configs/config.example.yaml configs/config.local.yaml
```

敏感值只通过环境变量注入，不要写入 YAML、源码、日志或提交记录：

```bash
export CONFIG_FILE=configs/config.local.yaml
export DATABASE_URL='postgres://personknow:personknow@127.0.0.1:5432/personknow?sslmode=disable'
export JWT_SECRET='替换为至少32字符的独立随机值'
export IDENTITY_SECRET='替换为另一个至少32字符的独立随机值'
export WECHAT_APP_ID='wx...'
export WECHAT_APP_SECRET='...'
export LLM_API_KEY='...'
```

可选环境变量包括 `DATA_DIR`、`ACCESS_TOKEN_TTL`、`REFRESH_TOKEN_TTL`、`WORKER_LEASE`、`WORKER_POLL` 和 `MILVUS_PASSWORD`。

### 4. 初始化并启动后端

```bash
go mod download
go run ./cmd/migrate up
go run ./cmd/migrate status
go run ./cmd/gateway
```

网关会同时启动 HTTP 服务和异步索引 Worker。浏览器打开 `http://127.0.0.1:8080`，健康检查为 `GET /api/health`，完整业务 API 位于 `/api/v1`。小程序联调前还应访问 `GET /api/v1/status`，确认当前端口不是仅包含旧接口的历史进程。

```bash
curl --fail http://127.0.0.1:8080/api/health
curl --fail http://127.0.0.1:8080/api/v1/status
```

### 5. 启动 Web 开发模式

另开终端，保留 Go 网关运行：

```bash
cd web
npm ci
npm run dev
```

访问 `http://127.0.0.1:5173`。Vite 会把 `/api` 请求代理到 `http://127.0.0.1:8080`。

### 6. 启动微信小程序

1. 用微信开发者工具导入 `miniprogram` 目录；
2. 本地预览可使用测试 AppID，登录联调需要真实 AppID/AppSecret；
3. 开发阶段可关闭合法域名校验；
4. 在“我的空间”把服务地址改为网关地址；
5. 真机不能使用 `127.0.0.1`，需填写手机可访问的局域网地址或 HTTPS 域名。

详细说明见 [miniprogram/README.md](miniprogram/README.md)。

## 前端开发

```bash
cd web
npm ci
npm test -- --run
npm run build
```

Web 构建产物输出到 `internal/gateway/web/dist` 并嵌入 Go 服务。小程序导入与发布说明见 [miniprogram/README.md](miniprogram/README.md)。

## 日志与审计

- 运行日志：结构化 JSON 写入 `logs/app.log`，按大小滚动并按天保留；
- 访问日志：包含请求 ID、状态码、耗时和资源标识，便于串联排障；
- 审计日志：账号、知识库、文件、索引和迁移等写操作进入 PostgreSQL 的 `audit_logs`；
- 脱敏规则：令牌、密钥、OpenID 明文和文档正文不会写入日志。

本地查看运行日志：

```bash
tail -f logs/app.log
```

## 线上部署

以下示例使用 `/srv/personknow`、systemd 和 Nginx；容器平台可使用同样的构建、migration、环境变量与健康检查顺序。

### 1. 构建发布产物

Web 产物会嵌入 Go 二进制，因此必须先构建 Web：

```bash
cd web
npm ci
npm test -- --run
npm run build
cd ..
go test ./...
go build -o bin/personknow ./cmd/gateway
go build -o bin/personknow-migrate ./cmd/migrate
```

将仓库或上述产物上传到服务器。生产运行不依赖 Node.js，也不需要单独部署 Web 静态目录。

### 2. 创建运行目录和配置

先创建无登录权限的服务账号（若服务器尚未创建）：

```bash
sudo useradd --system --home /srv/personknow --shell /usr/sbin/nologin personknow
```

再安装二进制、配置和运行目录：

```bash
sudo install -d -m 0755 -o personknow -g personknow /srv/personknow/bin
sudo install -d -m 0750 -o personknow -g personknow /srv/personknow/configs
sudo install -d -m 0750 -o personknow -g personknow /srv/personknow/data
sudo install -d -m 0750 -o personknow -g personknow /srv/personknow/logs
sudo install -d -m 0750 -o personknow -g personknow /srv/personknow/backups
sudo install -m 0755 bin/personknow /srv/personknow/bin/personknow
sudo install -m 0755 bin/personknow-migrate /srv/personknow/bin/personknow-migrate
sudo install -m 0640 -o personknow -g personknow configs/config.example.yaml /srv/personknow/configs/config.local.yaml
```

把敏感环境变量保存到仅部署账号和 `personknow` 服务账号可读的 `/etc/personknow.env`，权限设为 `0600`。生产数据库连接建议启用 TLS：

```bash
CONFIG_FILE=/srv/personknow/configs/config.local.yaml
DATABASE_URL=postgres://personknow:password@postgres:5432/personknow?sslmode=require
JWT_SECRET=替换为至少32字符的独立随机值
IDENTITY_SECRET=替换为另一个至少32字符的独立随机值
WECHAT_APP_ID=wx...
WECHAT_APP_SECRET=...
LLM_API_KEY=...
DATA_DIR=/srv/personknow/data
```

### 3. 执行数据库 migration

```bash
set -a
source /etc/personknow.env
set +a
/srv/personknow/bin/personknow-migrate up
/srv/personknow/bin/personknow-migrate status
```

发布时先 migration 再启动新版本；执行前应备份 PostgreSQL、`DATA_DIR` 和向量数据。

### 4. 使用 systemd 托管

创建 `/etc/systemd/system/personknow.service`：

```ini
[Unit]
Description=PersonKnow Gateway
After=network-online.target postgresql.service

[Service]
Type=simple
User=personknow
Group=personknow
WorkingDirectory=/srv/personknow
EnvironmentFile=/etc/personknow.env
ExecStart=/srv/personknow/bin/personknow
Restart=on-failure
RestartSec=3
TimeoutStopSec=20

[Install]
WantedBy=multi-user.target
```

加载并启动：

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now personknow
sudo systemctl status personknow
```

### 5. 配置 HTTPS 与 SSE 反向代理

Nginx 需要关闭代理缓冲并提高读取超时，否则流式问答会被缓存：

```nginx
location / {
    proxy_pass http://127.0.0.1:8080;
    proxy_set_header Host $host;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    proxy_set_header X-Forwarded-Proto $scheme;
    proxy_buffering off;
    proxy_read_timeout 300s;
}
```

由 Nginx/Caddy 终止 HTTPS，只向公网开放 80/443；Go、PostgreSQL 和 Milvus 应运行在内网。

### 6. 发布后检查

```bash
curl --fail https://your-domain.example/api/health
sudo journalctl -u personknow -n 100 --no-pager
tail -n 100 /srv/personknow/logs/app.log
```

随后抽查登录、知识库列表、文件上传、重新索引、流式问答、引用、用量和审计日志。小程序还需在微信公众平台配置 `request`、`uploadFile`、`downloadFile` HTTPS 合法域名，并用真机验证。

完整的权限、备份、恢复、Milvus 与旧数据迁移说明见 [docs/deploy-notes.md](docs/deploy-notes.md)。

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

## 开发约定

- 对外接口、复杂业务规则、边界条件和非直观实现必须写清晰注释；
- 注释优先解释设计原因、约束和异常处理，不重复代码字面含义；
- 修改行为时同步更新测试、README 和部署文档，避免文档与实现漂移。
