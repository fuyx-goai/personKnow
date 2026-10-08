# 个人知识库全栈版本设计说明

## 目标

在现有 Go、Web 和微信小程序项目上实现严格对应 Figma 原型的完整产品：微信与 Web 扫码同账号登录、多知识库、七类文件全生命周期、版本化异步索引、可溯源问答、容量与 Token 配额、RAG 微调、账号安全和审计日志。

## 已确认架构

- 单实例模块化 Go 单体，同时运行 Gin API 和 PostgreSQL 持久化 Worker。
- PostgreSQL 是账号、权限、文件状态、索引状态、用量和审计的事实源。
- 本地 UUID 目录保存原文件和解析/编辑文本版本。
- Mem 与 Milvus 只保存向量，通过用户、知识库、文档和内容版本元数据过滤。
- 小程序与 Web 使用 `/api/v1`，现有 `/api` 暂时保留兼容。
- 默认 Personal Pro 套餐为 5 GiB 和每月 2,000,000 Tokens。

## 原型约束

- 保持问答对答、知识库体系、原资料操作、我的空间四个入口。
- “我的空间”三个页签全部接入真实能力：容量与用量、RAG 检索微调、账号与安全。
- 公开知识库在原型“全部”筛选中使用同类卡片展示，增加所有者和只读标记，不新增独立导航。
- 所有加载、空态、错误、配额和任务状态沿用原型视觉语言。

## 设计索引

- 产品需求：`fullstack-knowledge-base/PRD.md`
- 流程、时序、架构和状态：`fullstack-knowledge-base/diagrams.md`
- 技术方案、API 和实施顺序：`fullstack-knowledge-base/tech-plan.md`
- PostgreSQL 数据模型：`fullstack-knowledge-base/database.md`
- 两轮评审：`fullstack-knowledge-base/review-1.md`、`fullstack-knowledge-base/review-2.md`

## 质量门禁

- 两名用户的私有资源必须互不可见；公开库只能被其他用户读取和问答。
- 新索引完整写入后才切换有效版本，失败时旧版本继续服务。
- 配额在产生外部模型成本前校验，用量流水与月汇总在事务内一致更新。
- 关键操作必须生成审计记录，错误可通过 `request_id` 追踪。
- 日志禁止包含令牌、微信 code、OpenID 明文、密钥、文件正文和完整问答内容。
- 完成前必须通过 Go、Web、小程序、数据库集成和双用户端到端验证。
