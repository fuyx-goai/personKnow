# 个人知识库全栈版本 · 技术方案

## 1. 技术选型总览

| 层级 | 技术选型 | 理由 |
|------|---------|------|
| 微信端 | 原生小程序 WXML/WXSS/JavaScript | 保持现有工程和原型四页结构，不引入额外运行时 |
| Web | 复用现有 Vite 前端 | 已完成豆包式页面，可增量接入账号和多库 API |
| API | Go 1.24 + Gin | 延续现有网关，适合 SSE 与单实例部署 |
| 业务结构 | 模块化单体 | 满足 100 用户、10 并发，同时保留清晰拆分边界 |
| 数据库 | PostgreSQL + pgx/v5 | 支持事务、行锁、JSONB、`SKIP LOCKED` 和高质量 Go 驱动 |
| 迁移 | 嵌入式版本化 SQL migration | 启动前显式执行，结构变更可追踪且无 ORM 隐式行为 |
| 鉴权 | 微信 code2session + JWT + 可撤销刷新会话 | 支持小程序与 Web 扫码同账号登录 |
| 文件存储 | 本地 UUID 隔离目录 | 符合已确认单实例方案，防止文件名路径穿越 |
| 异步任务 | PostgreSQL `FOR UPDATE SKIP LOCKED` | 无需 Redis，任务可持久化并支持重启恢复 |
| 向量 | 现有 Mem / Milvus 双实现 | 保留开发便利与生产向量检索能力 |
| AI 编排 | 现有 Eino + 模型适配器 | 最大限度复用当前聊天和 Embedding 能力 |
| 日志 | `log/slog` JSON + 滚动文件 | Go 标准结构化日志，支持 request_id 和敏感字段控制 |
| 测试 | Go testing + httptest + PostgreSQL 集成测试 | 覆盖领域、事务、API、Worker 与向量契约 |

## 2. 方案对比

### 方案 A：模块化单体 + PostgreSQL 任务队列

**适用场景**：单实例、约 100 用户、10 个并发问答。

**优点**：部署单元少；可复用现有 Go 入口；任务持久化；无需 Redis。

**缺点**：API 与 Worker 共享进程资源；本地文件限制多实例扩展。

**实现复杂度**：中。

### 方案 B：API 与 Worker 双进程

**适用场景**：索引量增加但仍使用共享 PostgreSQL 与文件目录。

**优点**：任务与在线请求资源隔离，Worker 可独立重启。

**缺点**：部署和进程管理更复杂，本地文件仍需共享挂载。

**实现复杂度**：中高。

### 方案 C：Redis 队列 + 对象存储 + 独立服务

**适用场景**：多实例、高并发和跨节点文件访问。

**优点**：横向扩展能力最强。

**缺点**：偏离本期已确认的单实例和本地存储约束，运维成本高。

**实现复杂度**：高。

### 方案对比矩阵

| 维度 | 方案 A | 方案 B | 方案 C | 权重 |
|------|--------|--------|--------|------|
| 当前规模匹配 | ★★★★★ | ★★★★☆ | ★★☆☆☆ | 高 |
| 部署简单度 | ★★★★★ | ★★★☆☆ | ★☆☆☆☆ | 高 |
| 任务可靠性 | ★★★★☆ | ★★★★★ | ★★★★★ | 高 |
| 横向扩展 | ★★☆☆☆ | ★★★☆☆ | ★★★★★ | 中 |
| 实施成本 | ★★★★☆ | ★★★☆☆ | ★☆☆☆☆ | 高 |

### ✅ 推荐方案：方案 A

**推荐理由**：

1. 与已确认的单实例、PostgreSQL、本地文件和首版规模完全匹配。
2. 使用模块端口隔离文件、任务与向量实现，未来可平滑演进到方案 B 或 C。

**注意事项/风险**：

- API 与 Worker 争用 CPU：Worker 使用可配置并发数，默认 2，并限制单任务批量。
- 本地文件不可跨节点：通过 `FileStore` 接口隔离，第二期可替换对象存储。
- PostgreSQL 任务轮询压力：使用状态与调度时间联合索引，并采用短事务领取。

## 3. 目标代码结构

```text
cmd/
├── gateway/             HTTP 与 Worker 组合启动
└── migrate/             数据库与旧向量迁移命令
internal/
├── account/             微信登录、会话、扫码票据
├── library/             知识库与访问策略
├── document/            文档元数据、内容版本、FileStore
├── indexing/            任务状态机、Worker、解析与向量切换
├── chat/                会话、消息、检索范围、SSE
├── usage/               配额、流水、月汇总、审计
├── platform/
│   ├── postgres/        pgx 连接、事务与 migration
│   ├── logging/         slog、轮转与敏感字段规则
│   └── httpx/           request_id、错误与鉴权中间件
└── knowledge/           现有模型适配与渐进迁移
pkg/config/              配置结构和校验
```

每个模块按 `model / service / repository / transport` 的责任拆分，但避免为简单实体创建空壳层。公共共享类型只包含身份、分页和错误，不建立万能工具包。

## 4. 关键技术实现要点

### 4.1 配置与密钥

- 新增 PostgreSQL DSN、数据目录、JWT 签名密钥、微信 AppID/AppSecret、Worker 并发、日志目录与保留周期。
- 密钥优先从环境变量读取；YAML 只保存非敏感默认项。
- 启动日志仅输出脱敏配置摘要，禁止输出 DSN 密码、API Key 和 JWT 密钥。
- 启动时验证必需配置；生产模式缺少签名密钥或微信密钥时直接失败。

### 4.2 数据库与事务

- 使用 `pgxpool` 管理连接，所有 repository 方法显式接收 `context.Context`。
- 事务由 service 层定义边界，repository 不自行开启嵌套事务。
- 配额校验锁定对应 `monthly_usage` 行；写入用量流水与汇总必须同事务提交。
- 登录票据确认使用条件更新，确保只有 `pending` 状态能变为 `confirmed`。
- 索引任务领取使用 `FOR UPDATE SKIP LOCKED`，领取事务只更新状态和租约，不执行耗时解析。

### 4.3 认证与授权

- 访问令牌默认 15 分钟，刷新会话默认 30 天。
- 数据库只保存刷新令牌哈希；退出登录或异常使用时撤销会话。
- Web 二维码只编码随机票据，不包含用户 ID、OpenID 或令牌。
- `AccessPolicy` 根据当前用户、知识库所有者和可见性返回 `owner/read/none`。
- 他人私有资源统一返回 404；公开库写请求返回 403。

### 4.4 文件保存与解析

- 临时文件位于数据目录内，校验完成后使用原子 rename 进入正式目录。
- 实际路径由 `user_id/library_id/document_id` 和固定子目录组成，客户端文件名从不参与路径拼接。
- 使用 MIME、扩展名和 magic bytes 三重校验；允许 PDF、DOCX、MD、TXT、HTML、CSV、PPTX。
- 解析器统一输出 UTF-8 文本、页码/章节映射和基础摘要信息。
- PDF、DOCX、PPTX 编辑的是解析文本版本，原始二进制文件不变。
- 每次编辑生成不可变 `document_contents` 版本，并以 SHA-256 判断是否重复。

### 4.5 索引任务与向量版本

- 任务状态：`queued/running/ready/failed/cancelled`。
- Worker 保存租约时间和心跳；启动时回收超过租约的 `running` 任务。
- 自动重试最多 3 次，使用指数退避；权限、格式和配额错误不可自动重试。
- 向量 ID 包含文档、内容版本和片段序号，避免名称冲突。
- 新版本向量写完后，在 PostgreSQL 事务内切换 `active_content_version_id`。
- 检索只接受 `SearchScope`，包含允许的知识库 ID 和有效版本条件。
- Mem 与 Milvus 实现相同的保存、过滤检索、按版本删除和计数契约。

### 4.6 问答、引用与 Token

- 创建会话时保存 `single_library/global` 模式；单库会话绑定知识库 ID。
- 全域范围由服务端计算：自己的可用知识库加其他用户公开知识库。
- SSE 事件固定为 `delta/reference/usage/error/done`，客户端按类型更新原型界面。
- 引用保存文档、片段、页码/章节、相似度和当时的内容版本。
- 调用模型前预检月度额度；调用后用供应商返回值或统一 tokenizer 记录实际 Token。
- 客户端中断后取消上下文，但仍保存供应商已返回的实际用量与中断消息。

### 4.7 用量与配额

- 存储用量按当前未删除原文件及文本版本字节数计算。
- Token 类型分为 `embedding/input/output`，全部写入不可变 `usage_records`。
- `monthly_usage` 是按用户和 UTC 月份的事务聚合，用于低延迟配额校验。
- 默认 Personal Pro：5 GiB、2,000,000 Tokens/月。
- 定时校准任务从流水重算聚合，发现差异时记录告警，不直接静默覆盖。

### 4.8 日志与审计

- Gin 中间件生成或透传安全格式的 `request_id`，响应头和错误体同步返回。
- `slog` 字段包括时间、级别、request_id、user_id、路由、状态、耗时和错误分类。
- Worker 字段包括 job_id、document_id、stage、attempt、duration 和 result。
- `audit_logs` 保存主体、操作、资源类型、资源 ID、结果、request_id 和最小化 metadata。
- 禁止记录微信 code、JWT、刷新令牌、OpenID 明文映射、密钥、问题全文、回答全文和文件正文。
- 日志默认单文件 100 MB、保留 30 天；审计记录保留 180 天。

### 4.9 API 契约

- 新接口统一前缀 `/api/v1`，错误格式为 `code/message/request_id/details`。
- 列表接口使用 `cursor/limit`；原型当前页可先使用默认 20 条并支持加载更多。
- 写接口支持 `Idempotency-Key`，服务端按用户、路由和键保存短期结果。
- 旧 `/api` 接口作为兼容适配层，在迁移验证通过前继续工作。

### 4.10 原型约束

- 小程序保持问答对答、知识库体系、原资料操作、我的空间四个入口。
- 所有按钮、状态、统计和来源卡片必须接入真实 API，不保留“后端暂不支持”占位提示。
- 公开库只读态必须隐藏或禁用编辑、重新索引和删除入口。
- 加载、空态、失败、配额超限和任务状态使用原型现有视觉语言。
- 知识库页“全部”筛选中先展示我的知识库，再展示公开知识库；公开卡片沿用原型样式并增加所有者和只读标识。

### 4.11 RAG 参数与账号安全

- 检索配置按知识库存储，默认 `chunk_size=800`、`chunk_overlap=100`、`top_k=5`、`similarity_threshold=0.30`。
- 修改 `top_k` 或相似度阈值后立即用于新问答；修改切块大小或重叠量会创建库级重建任务。
- 库级重建按文档逐一创建索引任务；所有文档成功前保留各自旧版本，失败文档可单独重试。
- 账号安全读取 `auth_sessions` 的脱敏设备信息；撤销会话只更新 `revoked_at`，审计记录操作主体和目标会话。
- 内容版本首版仅后台保留最近 10 个，不在原型中新增版本历史入口。

## 5. API 清单

| 模块 | 方法与路径 | 权限 |
|------|------------|------|
| Auth | `POST /api/v1/auth/wechat/login` | 公开 |
| Auth | `POST /api/v1/auth/refresh` | 刷新会话 |
| Auth | `POST /api/v1/auth/logout` | 登录用户 |
| Auth | `POST /api/v1/auth/web/tickets` | 公开 |
| Auth | `GET /api/v1/auth/web/tickets/:id` | 票据持有者 |
| Auth | `POST /api/v1/auth/web/tickets/:id/confirm` | 小程序登录用户 |
| User | `GET /api/v1/me` | 登录用户 |
| User | `GET /api/v1/me/sessions` | 登录用户 |
| User | `DELETE /api/v1/me/sessions/:id` | 会话所有者 |
| User | `DELETE /api/v1/me/sessions` | 登录用户，退出其他设备 |
| Library | `GET/POST /api/v1/libraries` | 登录用户 |
| Library | `GET/PATCH/DELETE /api/v1/libraries/:id` | 读取者/所有者 |
| Library | `GET /api/v1/libraries/public` | 登录用户 |
| Library | `GET/PATCH /api/v1/libraries/:id/retrieval-settings` | 读取者/所有者 |
| Library | `POST /api/v1/libraries/:id/reindex` | 所有者 |
| Document | `GET/POST /api/v1/documents` | 读取者/所有者 |
| Document | `GET/PATCH/DELETE /api/v1/documents/:id` | 读取者/所有者 |
| Document | `GET /api/v1/documents/:id/content` | 读取者 |
| Index | `POST /api/v1/documents/:id/reindex` | 所有者 |
| Index | `GET /api/v1/index-jobs/:id` | 所有者 |
| Index | `POST /api/v1/index-jobs/:id/retry` | 所有者 |
| Chat | `GET/POST /api/v1/chat/sessions` | 登录用户 |
| Chat | `GET/DELETE /api/v1/chat/sessions/:id` | 会话所有者 |
| Chat | `POST /api/v1/chat/sessions/:id/messages/stream` | 会话所有者 |
| Usage | `GET /api/v1/usage/summary` | 登录用户 |
| Usage | `GET /api/v1/usage/records` | 登录用户 |
| Audit | `GET /api/v1/audit-logs` | 登录用户，仅本人 |

## 6. 测试与验证策略

- 单元测试覆盖权限矩阵、状态机、配额、UTC 月份、文件路径和错误映射。
- PostgreSQL repository 集成测试覆盖事务、行锁、票据消费、任务领取和用量并发。
- 所有向量实现运行同一套契约测试，验证过滤、版本切换和删除语义。
- API 使用 `httptest` 覆盖登录替身、公开访问、越权、上传、索引、SSE 和配额错误。
- Worker 使用固定测试文件覆盖七类解析器、失败重试和重启恢复。
- 小程序继续使用 Node 测试工具函数、`node --check` 和 JSON/WXML 静态校验。
- Web 运行现有构建，并新增扫码登录和 API 映射测试。
- 最终执行 `go test ./...`、前端构建、静态检查和双用户端到端冒烟测试。

## 7. 项目排期估算（参考）

| 阶段 | 内容 | 工作量（人天） |
|------|------|--------------|
| 基础设施 | PostgreSQL、migration、配置、日志、错误体系 | 3 |
| 账号体系 | 微信登录、JWT、刷新会话、Web 扫码 | 4 |
| 多知识库 | 数据模型、权限、公开访问、统计 | 3 |
| 文件管理 | 本地存储、七类解析、版本与 API | 5 |
| 异步索引 | Worker、任务状态机、向量双实现改造 | 5 |
| 问答改造 | 检索范围、会话、引用、SSE 与用量 | 4 |
| 客户端联调 | 小程序与 Web 严格映射原型 | 5 |
| 迁移与验证 | 旧数据迁移、安全、集成与冒烟测试 | 4 |
| **合计** | | **33** |

## 8. 实施顺序

1. 建立配置、日志、错误、PostgreSQL 和 migration 基座。
2. 实现账号与会话，使后续 API 全部有用户上下文。
3. 实现知识库、访问策略和公开只读能力。
4. 实现 FileStore、文档版本和上传 API。
5. 改造向量端口，完成 Worker 与版本化索引。
6. 改造问答、会话、引用和用量事务。
7. 按原型接入小程序与 Web。
8. 执行旧数据迁移、全链路验证和安全检查。
