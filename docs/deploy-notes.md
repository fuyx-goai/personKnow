# PersonKnow 部署与恢复

## 1. 运行依赖

- Go 1.24；
- PostgreSQL 15 或更高版本，并允许创建 `pg_trgm` 扩展；
- Node.js 20 或更高版本仅用于重建 Web；
- 可选 Milvus 2.4；默认 `mem` 模式将向量写入迁移源或运行目录中的 `knowledge.json`。

生产环境建议由 Nginx/Caddy 终止 HTTPS，仅向外开放 80/443，并让 Go、PostgreSQL、Milvus 运行在内网。

## 2. 配置

复制 `configs/config.example.yaml` 为私有配置文件，并通过 `CONFIG_FILE` 指向它。数据库、认证、模型、存储、Worker 和日志参数均写入该 YAML：

```yaml
vector_store: mem
database:
  url: "postgres://personknow:password@postgres:5432/personknow?sslmode=require"
auth:
  jwt_secret: "至少32字符的独立随机值"
  identity_secret: "至少32字符的独立随机值"
  wechat_app_id: "wx..."
  wechat_app_secret: "..."
storage:
  data_dir: "/srv/personknow/data"
llm:
  api_key: "..."
```

`CONFIG_FILE` 仅选择配置文件。私有 YAML 必须设为 `0640` 或更严格权限，不得提交到 Git，也不得在启动日志中打印密钥。

## 3. 目录与权限

```bash
install -d -m 0750 -o personknow -g personknow /srv/personknow/data
install -d -m 0750 -o personknow -g personknow /srv/personknow/logs
install -d -m 0750 -o personknow -g personknow /srv/personknow/backups
```

应用用户需要：

- `storage.data_dir` 指向目录的读写权限，用于原文件与内容版本；
- `log.dir` 的读写权限；
- 旧数据迁移时对 `--source` 的读写权限和 `--backup-dir` 的写权限；
- PostgreSQL schema 的 DDL/DML 权限。

## 4. 初始化与启动

```bash
go build -o bin/personknow ./cmd/gateway
go build -o bin/personknow-migrate ./cmd/migrate
CONFIG_FILE=configs/config.local.yaml bin/personknow-migrate up
CONFIG_FILE=configs/config.local.yaml bin/personknow-migrate status
CONFIG_FILE=configs/config.local.yaml bin/personknow
```

`cmd/gateway` 启动时也会检查 migration，但生产发布仍应先显式执行 `up`。建议由 systemd、容器编排器或进程守护工具管理，并设置 15 秒以上优雅停止时间。

## 5. 健康检查与反向代理

```bash
curl --fail http://127.0.0.1:8080/api/health
```

反向代理需保留 `Host`、`X-Forwarded-For`、`X-Forwarded-Proto`。问答使用 SSE，必须关闭代理缓冲并提高读取超时，例如 Nginx 的 `proxy_buffering off; proxy_read_timeout 300s;`。

## 6. 微信域名

在微信公众平台为生产小程序配置 HTTPS 合法域名：

- `request`：API、登录、SSE 与普通请求；
- `uploadFile`：文件上传；
- `downloadFile`：备份或文件下载。

域名证书链必须完整，不能使用 IP、明文 HTTP 或自签名证书。发布前恢复微信开发者工具的合法域名校验，并用真实 AppID 真机验证登录、上传、流式问答和扫码确认。

## 7. 日志与审计

服务输出结构化 JSON 日志，默认写入 `logs`，按 `log.max_size_mb` 滚动并保留 `log.retain_days` 天。日志只记录 request、worker、模型与资源标识，不记录令牌、密钥、OpenID 明文或文档正文。

账号、知识库、文件、索引和旧数据迁移的写操作同时进入 `audit_logs`。定期检查失败结果、异常请求量和长期运行的索引任务。

## 8. 旧 `knowledge.json` 迁移

```bash
bin/personknow-migrate legacy \
  --target-user '<user-uuid>' \
  --source '/srv/personknow/legacy/knowledge.json' \
  --backup-dir '/srv/personknow/backups'
```

行为保证：

- 先生成权限为 `0600` 的时间戳备份；
- 为目标用户创建或复用“默认知识库”；
- 按 `_source`/`_file_name` 分组，以来源和内容哈希生成稳定文档 ID；
- 写入内容版本、范围化向量元数据、空间用量和审计日志；
- 重复执行时已完成文档的新增文档数和片段数均为 0；
- 失败报告只含来源名、固定错误码和固定消息，不含正文。

`mem` 模式会在源 JSON 中追加范围化向量记录，因此必须保留备份；旧记录仍保留用于兼容，新的多用户检索只读取范围化记录。`milvus` 模式不会修改源 JSON，只将范围化向量写入 Milvus。

## 9. 备份与恢复

备份范围：PostgreSQL、`storage.data_dir` 指向的目录、Mem 模式的 `knowledge.json` 或 Milvus collection、私有配置的安全副本。数据库与文件目录应在同一维护窗口生成快照。

恢复顺序：

1. 停止网关与 Worker；
2. 恢复 PostgreSQL；
3. 恢复 `storage.data_dir` 指向的目录与向量数据；
4. 运行 `personknow-migrate status`，必要时执行 `up`；
5. 启动服务并检查 `/api/health`；
6. 抽查登录、知识库列表、文档内容、问答引用、用量与审计日志。

迁移回滚时先停服务，再恢复命令生成的 `knowledge.json.*.bak`、数据库快照和 `storage.data_dir` 快照，避免只回滚其中一项造成版本不一致。
