# PersonKnow 项目规则

## 后端目录约束

后端目录参考 [BwCloudWeGo/bw-cli](https://github.com/BwCloudWeGo/bw-cli) 的模块化单体结构。新增或移动 Go 代码时，必须遵守以下边界：

```text
cmd/<app>/                         进程入口与依赖装配
internal/gateway/
  client/                          外部服务客户端与网关适配器
  handler/                         兼容接口 HTTP 控制器
  request/                         兼容接口 HTTP 入参 DTO
  router/                          路由、中间件与静态资源挂载
  web/                             Web 构建产物嵌入
internal/<domain>/
  dto/                             业务命令、查询和响应 DTO
  entity/                          领域实体、端口与仓储接口
  handler/                         领域 HTTP 控制器
  model/                           持久化或外部模型映射
  repo/                            PostgreSQL、文件、向量等仓储实现
  service/                         用例编排、业务规则和异步处理
internal/platform/<component>/    可复用基础设施，不放业务规则
pkg/<component>/                   可被多个进程复用的公共基础包
```

### 强制规则

1. `internal/<domain>/` 根目录不得放 `.go` 业务实现文件；代码必须归入 `dto`、`entity`、`handler`、`model`、`repo` 或 `service`。不适用的层可以不创建，但不得用根目录文件替代。
2. `handler` 只负责协议绑定、鉴权上下文和响应序列化；不得直接写 SQL、读文件或调用第三方模型。
3. `service` 只依赖 `entity`/`dto` 定义的端口和领域类型；具体 PostgreSQL、文件系统、Milvus 实现在 `repo`。
4. `repo` 负责基础设施适配，不向 HTTP 层泄漏数据库行、Gin 类型或请求对象。
5. `router` 只能在 `internal/gateway/router` 组装；新增 API 必须同时更新路由、请求 DTO、处理器测试和接口文档。
6. `cmd` 只做配置读取、依赖注入和进程生命周期管理；不得承载业务规则。
7. 依赖方向保持 `gateway/handler -> domain/service -> domain/entity`，实现细节从 `repo` 注入；禁止反向依赖 `handler` 或 `router`。
8. 同一层的文件使用稳定命名：HTTP 入口优先使用 `handler/server.go`，业务入口优先使用 `service/service.go`，仓储实现使用 `repo/*_repository.go`。
9. 子目录包名必须与目录名一致，即 `package entity`、`package handler`、`package repo`、`package service`；跨层引用优先使用显式别名（如 `accountentity`、`accountservice`），避免新增 dot import。
10. 每个新增业务模块必须补充对应目录说明、单元测试和必要的迁移/部署文档；复杂规则注释解释“为什么”，不重复代码字面含义。

### 变更门禁

- 重排目录后必须执行 `gofmt -w`、`go test ./...` 和 `go vet ./...`。
- 修改 Web 或小程序时，继续执行根 README 中列出的前端构建和小程序语法验证。
- 任何跨模块移动都必须同步更新 import、测试、README、架构图和计划文档。
- 未经明确确认，不得把业务包重新平铺回 `internal/` 根目录，也不得新建第二套并行目录约定。
