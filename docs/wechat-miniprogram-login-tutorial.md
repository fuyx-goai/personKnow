# 微信小程序登录教程：从 `wx.login` 到业务 Token

这篇教程面向第一次接触微信小程序登录的开发者，结合 PersonKnow 当前代码，按实际调用顺序解释：小程序怎样拿到临时 `code`、Go 后端怎样向微信换取 `openid`、系统怎样创建用户和会话，以及后续接口怎样携带 Token。

## 1. 先理解：微信登录到底在做什么

微信小程序登录不是“小程序把用户名和密码发给后端”。完整过程分成两次身份交换：

1. 小程序调用 `wx.login()`，从微信客户端获得一个短期、一次性的 `code`；
2. 小程序把 `code` 发给自己的 Go 后端；
3. Go 后端携带 `AppID`、`AppSecret` 和 `code` 请求微信 `jscode2session`；
4. 微信返回该用户在当前小程序下的 `openid`；
5. Go 后端根据 `openid` 查找或创建业务用户；
6. Go 后端签发自己的 `access_token` 和 `refresh_token`；
7. 小程序以后访问知识库接口时携带 `access_token`，不再把微信 `code` 当作登录态使用。

```mermaid
sequenceDiagram
    participant Mini as 微信小程序
    participant WeChat as 微信服务器
    participant API as PersonKnow Go 网关
    participant DB as PostgreSQL

    Mini->>WeChat: wx.login()
    WeChat-->>Mini: 临时 code
    Mini->>API: POST /api/v1/auth/wechat/login<br/>携带 code
    API->>WeChat: GET /sns/jscode2session<br/>AppID + AppSecret + code
    WeChat-->>API: openid / unionid
    API->>DB: 查找或创建用户<br/>创建登录会话
    DB-->>API: 用户与会话
    API-->>Mini: access_token + refresh_token + user
    Mini->>API: Authorization: Bearer access_token
    API-->>Mini: 受保护业务数据
```

这里最重要的安全原则是：**`AppSecret` 只能保存在后端配置文件中，不能放进小程序代码，也不能由小程序直接请求微信 `jscode2session`。**

## 2. PersonKnow 中对应哪些代码

先认识调用链，后面排错时会非常方便：

```text
miniprogram/app.js
  启动时检查 fullstack 状态并恢复登录
        ↓
miniprogram/services/auth.js
  调用 wx.login，管理 access/refresh token
        ↓
miniprogram/services/api.js
  POST /api/v1/auth/wechat/login
        ↓
internal/gateway/router/v1_full.go
  注册登录、刷新、退出路由
        ↓
internal/account/handler/handler.go
  解析 JSON 请求并调用账号服务
        ↓
internal/account/service/service.go
  编排微信换码、用户创建、会话创建和审计
        ↓
internal/account/repo/wechat.go
  请求微信 jscode2session
        ↓
internal/account/repo/
  将用户、微信身份和会话写入 PostgreSQL
```

## 3. 为什么微信登录需要 PostgreSQL

PersonKnow 支持两种运行模式：

- `standalone`：不配置 PostgreSQL，使用 `mem` 向量库，可运行基础摄入、检索和问答接口；
- `fullstack`：配置 PostgreSQL，启用微信登录、多知识库、文件任务、会话、用量和审计。

微信登录不只是向微信换取 `openid`。后端还需要保存以下状态：

- `openid` 对应哪个业务用户；
- 当前设备有哪些登录会话；
- `refresh_token` 是否有效或已经撤销；
- 用户是否被停用；
- 登录操作对应的审计记录。

因此，**当前可以先用 `standalone + mem` 启动项目，但要实际调通小程序登录，必须再配置 PostgreSQL，切换到 `fullstack` 模式。**

## 4. 第一步：准备微信小程序信息

你需要在微信公众平台准备：

- 小程序 `AppID`；
- 小程序 `AppSecret`；
- 微信开发者工具中导入项目所使用的 AppID。

项目文件 `miniprogram/project.config.json` 中的 `appid` 必须与后端配置的 `auth.wechat_app_id` 属于同一个小程序，否则微信服务器会拒绝用 `code` 换取身份。

不要把 `AppSecret` 写入以下位置：

- `miniprogram/project.config.json`；
- 任意小程序 JavaScript 文件；
- Git 仓库中的公开配置；
- URL 查询参数或前端日志。

## 5. 第二步：创建私有配置文件

复制示例配置：

```bash
cp configs/config.example.yaml configs/config.local.yaml
```

`configs/config.local.yaml` 已被 `.gitignore` 忽略，适合保存本地私有配置。PersonKnow 的运行参数都从 YAML 读取；`CONFIG_FILE` 只负责告诉程序使用哪一个文件。

先填写完整模式需要的配置：

```yaml
http_addr: ":8080"
vector_store: mem

database:
  url: "postgres://personknow:personknow@127.0.0.1:5432/personknow?sslmode=disable"
  max_conns: 10

auth:
  jwt_secret: "请替换成至少32字符的随机字符串"
  identity_secret: "请替换成另一段至少32字符的随机字符串"
  access_ttl: "15m"
  refresh_ttl: "720h"
  wechat_app_id: "wx开头的小程序AppID"
  wechat_app_secret: "微信公众平台中的AppSecret"

llm:
  api_key: "你的方舟API Key"
  base_url: "https://ark.cn-beijing.volces.com/api/v3"
  chat_model: "你的对话模型或接入点"
  embed_model: "你的向量模型或接入点"
  embed_api: "ark_multimodal"
```

几个字段的作用：

- `jwt_secret`：签名短期 `access_token`；
- `identity_secret`：加密数据库中的微信身份原文；
- `access_ttl`：访问令牌有效期，示例为 15 分钟；
- `refresh_ttl`：刷新令牌有效期，示例为 30 天；
- `wechat_app_id` 和 `wechat_app_secret`：仅供 Go 后端请求微信服务器；
- `vector_store: mem`：不依赖 Milvus，向量写入本地 `knowledge.json`。

## 6. 第三步：启动 PostgreSQL

如果本机已经安装 PostgreSQL，可以直接创建数据库：

```bash
createdb personknow
```

也可以用 Docker：

```bash
docker run --name personknow-postgres \
  -e POSTGRES_USER=personknow \
  -e POSTGRES_PASSWORD=personknow \
  -e POSTGRES_DB=personknow \
  -p 5432:5432 \
  -d postgres:16
```

确认数据库可以连接：

```bash
psql 'postgres://personknow:personknow@127.0.0.1:5432/personknow?sslmode=disable' -c 'select 1'
```

## 7. 第四步：执行 migration

PersonKnow 不会在登录时临时创建表。首次启动完整模式前，先创建用户、微信身份、会话和审计等表：

```bash
CONFIG_FILE=configs/config.local.yaml go run ./cmd/migrate up
CONFIG_FILE=configs/config.local.yaml go run ./cmd/migrate status
```

`status` 应显示 migration 已应用。如果这里失败，先不要打开小程序，因为登录必然无法保存用户和会话。

## 8. 第五步：启动 Go 网关

```bash
CONFIG_FILE=configs/config.local.yaml go run ./cmd/gateway
```

新开一个终端检查健康状态：

```bash
curl --fail http://127.0.0.1:8080/api/health
curl --fail http://127.0.0.1:8080/api/v1/status
```

完整模式的状态响应应包含：

```json
{
  "version": "v1",
  "mode": "fullstack",
  "capabilities": [
    "wechat_login",
    "libraries",
    "documents",
    "miniprogram"
  ]
}
```

如果返回 `"mode":"standalone"`，说明 `database.url` 仍为空，或当前启动使用了另一个配置文件。

## 9. 第六步：用微信开发者工具启动小程序

1. 打开微信开发者工具；
2. 选择“导入项目”；
3. 项目目录选择仓库中的 `miniprogram`；
4. 确认 AppID 与 `auth.wechat_app_id` 一致；
5. 本地开发阶段可暂时关闭“合法域名校验”；
6. 编译并打开任意页面。

小程序启动后，`miniprogram/app.js` 会先调用 `/api/v1/status`。确认后才执行登录，这样可以提前识别旧网关或未配置数据库的独立模式。

## 10. 登录请求是怎样发出的

小程序中的核心逻辑可以简化理解为：

```javascript
const loginResult = await wx.login()

const response = await request({
  url: 'http://127.0.0.1:8080/api/v1/auth/wechat/login',
  method: 'POST',
  data: {
    code: loginResult.code,
    device_label: '微信小程序',
  },
})
```

实际项目由 `miniprogram/services/auth.js` 和 `miniprogram/services/api.js` 封装，不需要页面重复写这段逻辑。

登录接口：

```http
POST /api/v1/auth/wechat/login
Content-Type: application/json
```

请求体：

```json
{
  "code": "wx.login 返回的临时 code",
  "nickname": "可选昵称",
  "avatar_url": "可选头像地址",
  "device_label": "微信小程序"
}
```

成功响应示例：

```json
{
  "user": {
    "id": "019...",
    "nickname": "微信用户",
    "status": "active"
  },
  "access_token": "eyJ...",
  "refresh_token": "随机刷新令牌",
  "expires_in": 900
}
```

## 11. 后端拿到 code 后做了什么

### 11.1 参数校验

`internal/account/handler/handler.go` 检查 JSON 中是否包含 `code`。缺失时返回：

```json
{
  "code": "INVALID_REQUEST",
  "message": "请提供微信登录 code",
  "request_id": "..."
}
```

### 11.2 向微信换取身份

`internal/account/repo/wechat.go` 请求：

```text
GET https://api.weixin.qq.com/sns/jscode2session
```

后端发送：

- `appid`：配置文件中的 `auth.wechat_app_id`；
- `secret`：配置文件中的 `auth.wechat_app_secret`；
- `js_code`：小程序刚刚提交的临时 `code`；
- `grant_type=authorization_code`。

微信成功后返回 `openid`。`openid` 是用户在当前小程序中的稳定标识，但不应原样写入日志。

### 11.3 创建业务用户

`internal/account/repo/repository_user.go` 会：

1. 加密并哈希 `openid`；
2. 用 `AppID + openid_hash` 查找已有用户；
3. 如果不存在，创建 `users` 和 `wechat_identities` 记录；
4. 如果已经存在，更新昵称、头像和最后登录时间。

### 11.4 创建业务会话

`internal/account/service/service.go` 创建：

- 一个短期 JWT `access_token`；
- 一个长期随机 `refresh_token`；
- 一条 PostgreSQL `auth_sessions` 会话记录；
- 一条登录审计记录。

数据库只保存 `refresh_token` 的哈希，不保存可直接使用的明文。

## 12. Token 为什么分成两种

### access token

- 有效期短，默认 15 分钟；
- 保存在小程序内存中；
- 每次访问受保护接口时放在请求头；
- 泄露后的有效窗口较短。

```http
Authorization: Bearer eyJ...
```

### refresh token

- 有效期长，默认 30 天；
- 保存在微信本地存储中；
- 只用于换取新的 access token；
- 每次刷新都会轮换，旧 refresh token 立即失效。

当业务接口返回 `401` 时，`miniprogram/services/api.js` 会调用：

```http
POST /api/v1/auth/refresh
Content-Type: application/json

{
  "refresh_token": "本地保存的刷新令牌"
}
```

刷新成功后，原请求只重试一次，避免登录失效时无限循环。

## 13. 怎样确认登录真的成功了

### 13.1 看微信开发者工具 Network

应该依次看到：

1. `GET /api/v1/status` 返回 200；
2. `POST /api/v1/auth/wechat/login` 返回 200；
3. `GET /api/v1/me` 返回当前用户；
4. `GET /api/v1/libraries` 返回知识库列表。

### 13.2 看小程序 Console

搜索前缀：

```text
[miniprogram-api]
```

日志包含：

- 请求方法；
- API 路径；
- HTTP 状态码；
- 请求耗时；
- `request_id`。

日志不会打印 `access_token`、`refresh_token`、微信 `code` 或请求正文。

### 13.3 看后端日志

```bash
tail -f logs/app.log
```

后端访问日志也包含 `request_id`。前后端使用同一个 ID，可以快速确认请求是否到达 Go 网关、走了哪个路由、最终返回什么状态。

## 14. 常见错误与解决方法

### 14.1 `404 page not found`

现象：登录请求返回纯文本 404。

原因通常是 8080 端口仍运行旧二进制，没有注册 `/api/v1/auth/wechat/login`。

处理：

```bash
lsof -nP -iTCP:8080 -sTCP:LISTEN
curl http://127.0.0.1:8080/api/v1/status
```

停止旧的 GoLand 或终端进程，然后从当前分支使用私有配置重新运行：

```bash
CONFIG_FILE=configs/config.local.yaml go run ./cmd/gateway
```

### 14.2 提示当前是独立模式

现象：小程序提示先配置 PostgreSQL。

原因：`database.url` 为空，网关处于 `standalone` 模式。

处理：填写 `configs/config.local.yaml` 中的 `database.url`，执行 migration，再重启网关。

### 14.3 `WECHAT_LOGIN_FAILED`

常见原因：

- 小程序 AppID 与后端 `wechat_app_id` 不一致；
- AppSecret 错误或已重置；
- `code` 已使用、已过期；
- 服务器无法访问 `api.weixin.qq.com`。

每次重试都应重新调用 `wx.login()` 获取新 `code`，不能重复发送旧值。

### 14.4 真机请求不到 `127.0.0.1`

开发者工具中的 `127.0.0.1` 指向电脑，但真机中的 `127.0.0.1` 指向手机自己。

真机调试时，把小程序服务地址改为：

- 手机可访问的电脑局域网地址，例如 `http://192.168.1.20:8080`；或
- 已配置到微信公众平台的 HTTPS 公网域名。

### 14.5 请求被合法域名校验拦截

开发阶段可以在微信开发者工具中暂时关闭合法域名校验。发布前必须：

1. 使用 HTTPS；
2. 在微信公众平台配置 `request` 合法域名；
3. 重新打开合法域名校验；
4. 用真机重新验证登录。

### 14.6 登录成功后业务接口返回 401

先检查请求头是否包含：

```http
Authorization: Bearer <access_token>
```

再检查刷新接口是否成功。如果 refresh token 已过期或被撤销，小程序会清空本地会话并重新执行微信登录。

## 15. 安全检查清单

上线前逐项确认：

- [ ] AppSecret 只存在于私有 YAML 和服务端内存；
- [ ] `configs/config.local.yaml` 没有提交到 Git；
- [ ] 私有配置文件权限为 `0640` 或更严格；
- [ ] `jwt_secret` 和 `identity_secret` 是两段不同的随机字符串；
- [ ] 生产数据库连接启用了 TLS；
- [ ] 小程序请求使用 HTTPS 合法域名；
- [ ] 日志不包含 Token、AppSecret、微信 code 或 OpenID 明文；
- [ ] access token 有较短有效期；
- [ ] refresh token 在刷新时轮换；
- [ ] 退出登录会撤销服务端会话；
- [ ] 登录、刷新和退出操作都有审计记录。

## 16. 推荐的学习顺序

如果想通过代码继续学习，建议按以下顺序阅读：

1. `miniprogram/app.js`：理解应用如何启动和恢复会话；
2. `miniprogram/services/auth.js`：理解 access/refresh token 生命周期；
3. `miniprogram/services/api.js`：理解请求、日志和 401 重试；
4. `internal/gateway/router/v1_full.go`：理解公开路由和鉴权路由；
5. `internal/account/handler/handler.go`：理解 HTTP 参数和响应；
6. `internal/account/service/service.go`：理解登录业务编排；
7. `internal/account/repo/wechat.go`：理解 `code` 换 `openid`；
8. `internal/account/repo/repository_user.go`：理解用户与微信身份落库；
9. `internal/account/service/token.go`：理解 JWT 和 refresh token。

完成以上步骤后，你就能完整解释一条微信登录请求是怎样从小程序进入 Go 网关，再经过微信服务器和 PostgreSQL，最后变成 PersonKnow 业务登录态的。
