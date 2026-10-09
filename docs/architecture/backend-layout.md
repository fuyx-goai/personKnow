# 后端目录架构

PersonKnow 的 Go 后端采用参考 bw-cli 的模块化单体结构。HTTP 网关和业务域分开，业务域内部按职责分层，便于在不影响 API 路由的情况下替换数据库、向量库或模型。

## 目录约定

```text
cmd/
  gateway/                         主 API 与索引 Worker 进程
  migrate/                         schema 与旧数据迁移命令
internal/
  gateway/                         handler/request/router/web
  account/                         dto/entity/handler/model/repo/service
  library/                         dto/entity/handler/model/repo/service
  document/                        dto/entity/handler/model/repo/service
  indexing/                        dto/entity/handler/model/repo/service
  chat/                            dto/entity/handler/model/repo/service
  usage/                           dto/entity/handler/model/repo/service
  migration/                       repo/service（命令专用业务域）
  knowledge/                       兼容旧 `/api` 的 RAG 域，保持已有 dto/model/repo/service
  platform/                        httpx/logging/postgres/vector 基础设施
```

## 请求链路

```text
Gin Router
  -> gateway/domain handler
  -> domain service
  -> entity/repository interface
  -> repo implementation
  -> PostgreSQL / FileStore / Mem / Milvus / LLM
```

`cmd/gateway` 是唯一装配车间：它创建 repo、service、handler，再交给 `internal/gateway/router` 注册。业务包不主动读取环境变量，也不自行创建数据库连接。

## 变更记录

### 2026-10-08 - 对齐 bw-cli 目录分层

- 变更内容：将账号、知识库、文档、索引、问答、用量和迁移代码从业务域根目录迁入 `entity/handler/repo/service`。
- 变更理由：原有平铺目录混合了 HTTP、领域、持久化和用例编排职责，随着全栈功能增加后难以定位依赖边界。
- 影响范围：仅调整 Go package 路径、依赖装配和测试位置；HTTP 路由、JSON 契约、数据库 schema 和业务行为保持不变。
- 决策依据：参考 bw-cli 的 `internal/gateway` 与 `internal/<domain>/{dto,entity,handler,model,repo,service}` 结构，并针对本项目不需要的空层进行裁剪。

## 兼容说明

`internal/knowledge` 是早期单库 `/api` 接口使用的 RAG 代码，暂时保留其现有四层命名；新功能必须进入对应业务域，不得继续向该兼容包或 `internal/` 根目录添加代码。
