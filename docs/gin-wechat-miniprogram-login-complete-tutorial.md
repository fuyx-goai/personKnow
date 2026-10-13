# Gin 实现微信小程序登录：脱离具体项目的完整教程

这是一篇可以独立阅读、独立运行的微信小程序登录教程，不依赖任何现有项目。你将从零搭建一个 Gin 后端，让微信小程序完成以下闭环：

1. 小程序调用 `wx.login()` 获取一次性 `code`；
2. Gin 后端用 `code` 请求微信 `jscode2session`；
3. 后端用 `openid` 创建或查找业务用户；
4. 后端签发短期 JWT `access_token` 和长期 `refresh_token`；
5. 小程序携带 `access_token` 访问受保护接口；
6. `access_token` 过期后，用 `refresh_token` 换取新令牌；
7. 用户退出时，后端撤销会话。

教程中的后端是一个可运行的教学项目，使用以下技术：

- Go 1.22 或更高版本；
- Gin；
- PostgreSQL 15+；
- `pgx` 数据库驱动；
- `golang-jwt/jwt`；
- 微信原生小程序。

> 本教程不会把 `AppSecret`、`session_key` 或 `openid` 返回给小程序。`AppSecret` 永远只保存在后端。

## 1. 先理解完整登录链路

微信小程序登录包含两套身份凭证：微信临时凭证和你自己的业务令牌。

```mermaid
sequenceDiagram
    participant Mini as 微信小程序
    participant WX as 微信服务器
    participant API as Gin 后端
    participant DB as PostgreSQL

    Mini->>WX: wx.login()
    WX-->>Mini: 一次性 code
    Mini->>API: POST /api/v1/auth/wechat/login
    API->>WX: GET /sns/jscode2session
    WX-->>API: openid + session_key
    API->>DB: 查找或创建业务用户
    API->>DB: 保存 refresh_token 哈希
    API-->>Mini: access_token + refresh_token
    Mini->>API: GET /api/v1/me<br/>Authorization: Bearer access_token
    API-->>Mini: 当前用户
```

几个容易混淆的概念：

- `code`：`wx.login()` 返回的短期一次性凭证，只能由后端拿去换身份；
- `openid`：用户在当前小程序 AppID 下的稳定标识，不是跨小程序通用账号；
- `session_key`：微信会话密钥，主要用于解密特定开放数据，不是你业务接口的登录 Token；
- `access_token`：本教程中由你的后端签发，用于访问业务接口；
- `refresh_token`：本教程中由你的后端生成，用于续期业务登录态；
- 微信接口中的 `access_token`：微信服务端 API 的调用凭证，与本教程的业务 `access_token` 不是同一个东西。

## 2. 准备微信小程序

在微信公众平台准备：

- 小程序 `AppID`；
- 小程序 `AppSecret`；
- 微信开发者工具；
- 一个使用相同 AppID 创建或导入的小程序项目。

`AppSecret` 只能配置在 Gin 服务端。不要把它放进：

- 小程序 JavaScript；
- `project.config.json`；
- Git 仓库；
- 前端请求参数；
- 日志或错误响应。

微信登录接口文档：<https://developers.weixin.qq.com/miniprogram/dev/OpenApiDoc/user-login/code2Session.html>

## 3. 创建后端项目

创建一个全新的目录：

```bash
mkdir gin-wechat-login
cd gin-wechat-login
go mod init example.com/gin-wechat-login
go get github.com/gin-gonic/gin
go get github.com/golang-jwt/jwt/v5
go get github.com/jackc/pgx/v5/stdlib
mkdir -p cmd/server migrations
```

最终目录只有几个文件：

```text
gin-wechat-login/
├── cmd/
│   └── server/
│       └── main.go
├── migrations/
│   └── 001_init.sql
├── go.mod
└── go.sum
```

为了让教程便于复制运行，后端代码集中在一个 `main.go`。正式项目可以再按 `config`、`wechat`、`auth`、`repository`、`handler` 和 `middleware` 拆包。

## 4. 启动 PostgreSQL

如果本机有 Docker，可以直接启动数据库：

```bash
docker run --name gin-wechat-postgres \
  -e POSTGRES_USER=wechat \
  -e POSTGRES_PASSWORD=wechat \
  -e POSTGRES_DB=wechat_login \
  -p 5432:5432 \
  -d postgres:16
```

Windows PowerShell 可以使用单行命令：

```powershell
docker run --name gin-wechat-postgres -e POSTGRES_USER=wechat -e POSTGRES_PASSWORD=wechat -e POSTGRES_DB=wechat_login -p 5432:5432 -d postgres:16
```

数据库连接串为：

```text
postgres://wechat:wechat@127.0.0.1:5432/wechat_login?sslmode=disable
```

## 5. 创建数据库表

新建 `migrations/001_init.sql`：

```sql
CREATE TABLE IF NOT EXISTS users (
    id BIGSERIAL PRIMARY KEY,
    appid VARCHAR(64) NOT NULL,
    openid_hash CHAR(64) NOT NULL,
    nickname VARCHAR(100) NOT NULL DEFAULT '微信用户',
    avatar_url TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (appid, openid_hash)
);

CREATE TABLE IF NOT EXISTS auth_sessions (
    id VARCHAR(64) PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    refresh_token_hash CHAR(64) NOT NULL UNIQUE,
    expires_at TIMESTAMPTZ NOT NULL,
    revoked_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_auth_sessions_user_id
    ON auth_sessions(user_id);

CREATE INDEX IF NOT EXISTS idx_auth_sessions_expires_at
    ON auth_sessions(expires_at);
```

执行 migration：

```bash
psql 'postgres://wechat:wechat@127.0.0.1:5432/wechat_login?sslmode=disable' \
  -f migrations/001_init.sql
```

这里没有保存 `openid` 明文，而是保存 `AppID + openid` 的 SHA-256 摘要。后端每次从微信获得 `openid` 后重新计算摘要，就能定位业务用户。

> 如果业务确实需要再次使用 `openid` 调用微信接口，应额外使用服务端密钥加密后保存，不能只做普通 Base64 编码。

## 6. 编写完整 Gin 后端

新建 `cmd/server/main.go`：

```go
package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	_ "github.com/jackc/pgx/v5/stdlib"
)

const jwtIssuer = "gin-wechat-login"

type config struct {
	ServerAddr     string
	DatabaseURL    string
	WeChatAppID    string
	WeChatSecret   string
	JWTSecret      []byte
	AccessTokenTTL time.Duration
	RefreshTTL     time.Duration
}

type application struct {
	cfg        config
	db         *sql.DB
	httpClient *http.Client
}

type user struct {
	ID        int64  `json:"id"`
	Nickname  string `json:"nickname"`
	AvatarURL string `json:"avatar_url"`
}

type weChatSession struct {
	OpenID     string `json:"openid"`
	SessionKey string `json:"session_key"`
	UnionID    string `json:"unionid"`
	ErrCode    int    `json:"errcode"`
	ErrMsg     string `json:"errmsg"`
}

type loginRequest struct {
	Code      string `json:"code" binding:"required"`
	Nickname  string `json:"nickname"`
	AvatarURL string `json:"avatar_url"`
}

type refreshRequest struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
}

type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int64  `json:"expires_in"`
}

func main() {
	cfg, err := loadConfig()
	if err != nil {
		log.Fatal(err)
	}

	db, err := sql.Open("pgx", cfg.DatabaseURL)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	pingCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := db.PingContext(pingCtx); err != nil {
		log.Fatalf("database unavailable: %v", err)
	}

	app := &application{
		cfg: cfg,
		db:  db,
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
		},
	}

	router := gin.New()
	router.Use(gin.Logger(), gin.Recovery())
	if err := router.SetTrustedProxies(nil); err != nil {
		log.Fatal(err)
	}

	router.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})
	router.POST("/api/v1/auth/wechat/login", app.weChatLogin)
	router.POST("/api/v1/auth/refresh", app.refresh)
	router.POST("/api/v1/auth/logout", app.logout)

	protected := router.Group("/api/v1")
	protected.Use(app.authenticate())
	protected.GET("/me", app.me)

	server := &http.Server{
		Addr:              cfg.ServerAddr,
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	go func() {
		log.Printf("server listening on %s", cfg.ServerAddr)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("server failed: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("graceful shutdown failed: %v", err)
	}
}

func loadConfig() (config, error) {
	accessTTL, err := time.ParseDuration(envOr("ACCESS_TOKEN_TTL", "15m"))
	if err != nil {
		return config{}, fmt.Errorf("invalid ACCESS_TOKEN_TTL: %w", err)
	}
	refreshTTL, err := time.ParseDuration(envOr("REFRESH_TOKEN_TTL", "720h"))
	if err != nil {
		return config{}, fmt.Errorf("invalid REFRESH_TOKEN_TTL: %w", err)
	}

	cfg := config{
		ServerAddr:     envOr("SERVER_ADDR", ":8080"),
		DatabaseURL:    os.Getenv("DATABASE_URL"),
		WeChatAppID:    os.Getenv("WECHAT_APP_ID"),
		WeChatSecret:   os.Getenv("WECHAT_APP_SECRET"),
		JWTSecret:      []byte(os.Getenv("JWT_SECRET")),
		AccessTokenTTL: accessTTL,
		RefreshTTL:     refreshTTL,
	}

	switch {
	case cfg.DatabaseURL == "":
		return config{}, errors.New("DATABASE_URL is required")
	case cfg.WeChatAppID == "":
		return config{}, errors.New("WECHAT_APP_ID is required")
	case cfg.WeChatSecret == "":
		return config{}, errors.New("WECHAT_APP_SECRET is required")
	case len(cfg.JWTSecret) < 32:
		return config{}, errors.New("JWT_SECRET must contain at least 32 characters")
	case cfg.AccessTokenTTL <= 0 || cfg.RefreshTTL <= 0:
		return config{}, errors.New("token TTL must be positive")
	}
	return cfg, nil
}

func envOr(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func (app *application) weChatLogin(c *gin.Context) {
	var input loginRequest
	if err := c.ShouldBindJSON(&input); err != nil || strings.TrimSpace(input.Code) == "" {
		writeError(c, http.StatusBadRequest, "INVALID_REQUEST", "请提供 wx.login 返回的 code")
		return
	}

	wxSession, err := app.exchangeCode(c.Request.Context(), input.Code)
	if err != nil {
		log.Printf("wechat code exchange failed: %v", err)
		writeError(c, http.StatusBadGateway, "WECHAT_LOGIN_FAILED", "微信登录失败，请重新获取 code")
		return
	}

	currentUser, err := app.upsertUser(c.Request.Context(), wxSession.OpenID, input)
	if err != nil {
		log.Printf("upsert user failed: %v", err)
		writeError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "创建用户失败")
		return
	}

	tokens, err := app.createSession(c.Request.Context(), currentUser.ID)
	if err != nil {
		log.Printf("create session failed: %v", err)
		writeError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "创建登录会话失败")
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"user":          currentUser,
		"access_token":  tokens.AccessToken,
		"refresh_token": tokens.RefreshToken,
		"token_type":    tokens.TokenType,
		"expires_in":    tokens.ExpiresIn,
	})
}

func (app *application) exchangeCode(ctx context.Context, code string) (weChatSession, error) {
	endpoint, err := url.Parse("https://api.weixin.qq.com/sns/jscode2session")
	if err != nil {
		return weChatSession{}, err
	}
	query := endpoint.Query()
	query.Set("appid", app.cfg.WeChatAppID)
	query.Set("secret", app.cfg.WeChatSecret)
	query.Set("js_code", code)
	query.Set("grant_type", "authorization_code")
	endpoint.RawQuery = query.Encode()

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return weChatSession{}, err
	}
	response, err := app.httpClient.Do(request)
	if err != nil {
		return weChatSession{}, errors.New("request wechat jscode2session failed")
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return weChatSession{}, fmt.Errorf("wechat returned HTTP %d", response.StatusCode)
	}

	var result weChatSession
	decoder := json.NewDecoder(io.LimitReader(response.Body, 1<<20))
	if err := decoder.Decode(&result); err != nil {
		return weChatSession{}, fmt.Errorf("decode wechat response: %w", err)
	}
	if result.ErrCode != 0 {
		return weChatSession{}, fmt.Errorf("wechat errcode=%d errmsg=%s", result.ErrCode, result.ErrMsg)
	}
	if result.OpenID == "" {
		return weChatSession{}, errors.New("wechat response does not contain openid")
	}
	return result, nil
}

func (app *application) upsertUser(ctx context.Context, openID string, input loginRequest) (user, error) {
	providedNickname := strings.TrimSpace(input.Nickname)
	nicknameToInsert := providedNickname
	if nicknameToInsert == "" {
		nicknameToInsert = "微信用户"
	}
	openIDHash := sha256.Sum256([]byte(app.cfg.WeChatAppID + ":" + openID))

	var currentUser user
	err := app.db.QueryRowContext(ctx, `
		INSERT INTO users (appid, openid_hash, nickname, avatar_url)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (appid, openid_hash) DO UPDATE SET
			nickname = CASE
				WHEN $5 = '' THEN users.nickname
				ELSE EXCLUDED.nickname
			END,
			avatar_url = CASE
				WHEN EXCLUDED.avatar_url = '' THEN users.avatar_url
				ELSE EXCLUDED.avatar_url
			END,
			updated_at = NOW()
		RETURNING id, nickname, avatar_url
	`, app.cfg.WeChatAppID, hex.EncodeToString(openIDHash[:]), nicknameToInsert, input.AvatarURL, providedNickname).Scan(
		&currentUser.ID,
		&currentUser.Nickname,
		&currentUser.AvatarURL,
	)
	return currentUser, err
}

func (app *application) createSession(ctx context.Context, userID int64) (tokenResponse, error) {
	sessionID, err := randomToken(18)
	if err != nil {
		return tokenResponse{}, err
	}
	refreshToken, err := randomToken(32)
	if err != nil {
		return tokenResponse{}, err
	}
	refreshExpiresAt := time.Now().Add(app.cfg.RefreshTTL)

	_, err = app.db.ExecContext(ctx, `
		INSERT INTO auth_sessions (id, user_id, refresh_token_hash, expires_at)
		VALUES ($1, $2, $3, $4)
	`, sessionID, userID, hashToken(refreshToken), refreshExpiresAt)
	if err != nil {
		return tokenResponse{}, err
	}

	accessToken, err := app.signAccessToken(userID, sessionID)
	if err != nil {
		return tokenResponse{}, err
	}
	return tokenResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		TokenType:    "Bearer",
		ExpiresIn:    int64(app.cfg.AccessTokenTTL.Seconds()),
	}, nil
}

func (app *application) refresh(c *gin.Context) {
	var input refreshRequest
	if err := c.ShouldBindJSON(&input); err != nil {
		writeError(c, http.StatusBadRequest, "INVALID_REQUEST", "请提供 refresh_token")
		return
	}

	tx, err := app.db.BeginTx(c.Request.Context(), nil)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "刷新失败")
		return
	}
	defer tx.Rollback()

	var sessionID string
	var userID int64
	err = tx.QueryRowContext(c.Request.Context(), `
		SELECT id, user_id
		FROM auth_sessions
		WHERE refresh_token_hash = $1
		  AND revoked_at IS NULL
		  AND expires_at > NOW()
		FOR UPDATE
	`, hashToken(input.RefreshToken)).Scan(&sessionID, &userID)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(c, http.StatusUnauthorized, "INVALID_REFRESH_TOKEN", "登录状态已失效")
		return
	}
	if err != nil {
		log.Printf("find refresh session failed: %v", err)
		writeError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "刷新失败")
		return
	}

	newRefreshToken, err := randomToken(32)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "刷新失败")
		return
	}
	newRefreshExpiresAt := time.Now().Add(app.cfg.RefreshTTL)
	_, err = tx.ExecContext(c.Request.Context(), `
		UPDATE auth_sessions
		SET refresh_token_hash = $1,
			expires_at = $2,
			updated_at = NOW()
		WHERE id = $3
	`, hashToken(newRefreshToken), newRefreshExpiresAt, sessionID)
	if err != nil {
		log.Printf("rotate refresh token failed: %v", err)
		writeError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "刷新失败")
		return
	}
	if err := tx.Commit(); err != nil {
		writeError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "刷新失败")
		return
	}

	accessToken, err := app.signAccessToken(userID, sessionID)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "刷新失败")
		return
	}
	c.JSON(http.StatusOK, tokenResponse{
		AccessToken:  accessToken,
		RefreshToken: newRefreshToken,
		TokenType:    "Bearer",
		ExpiresIn:    int64(app.cfg.AccessTokenTTL.Seconds()),
	})
}

func (app *application) logout(c *gin.Context) {
	var input refreshRequest
	if err := c.ShouldBindJSON(&input); err != nil {
		writeError(c, http.StatusBadRequest, "INVALID_REQUEST", "请提供 refresh_token")
		return
	}
	_, err := app.db.ExecContext(c.Request.Context(), `
		UPDATE auth_sessions
		SET revoked_at = COALESCE(revoked_at, NOW()), updated_at = NOW()
		WHERE refresh_token_hash = $1
	`, hashToken(input.RefreshToken))
	if err != nil {
		writeError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "退出失败")
		return
	}
	c.Status(http.StatusNoContent)
}

func (app *application) signAccessToken(userID int64, sessionID string) (string, error) {
	now := time.Now()
	claims := jwt.RegisteredClaims{
		Issuer:    jwtIssuer,
		Subject:   strconv.FormatInt(userID, 10),
		ID:        sessionID,
		IssuedAt:  jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(now.Add(app.cfg.AccessTokenTTL)),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(app.cfg.JWTSecret)
}

func (app *application) authenticate() gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		parts := strings.Fields(header)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			writeError(c, http.StatusUnauthorized, "UNAUTHORIZED", "缺少有效的 Bearer Token")
			c.Abort()
			return
		}

		claims := &jwt.RegisteredClaims{}
		token, err := jwt.ParseWithClaims(parts[1], claims, func(token *jwt.Token) (any, error) {
			return app.cfg.JWTSecret, nil
		}, jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}), jwt.WithIssuer(jwtIssuer))
		if err != nil || !token.Valid || claims.ID == "" {
			writeError(c, http.StatusUnauthorized, "UNAUTHORIZED", "访问令牌无效或已过期")
			c.Abort()
			return
		}

		userID, err := strconv.ParseInt(claims.Subject, 10, 64)
		if err != nil {
			writeError(c, http.StatusUnauthorized, "UNAUTHORIZED", "访问令牌无效")
			c.Abort()
			return
		}

		var active bool
		err = app.db.QueryRowContext(c.Request.Context(), `
			SELECT EXISTS (
				SELECT 1 FROM auth_sessions
				WHERE id = $1
				  AND user_id = $2
				  AND revoked_at IS NULL
				  AND expires_at > NOW()
			)
		`, claims.ID, userID).Scan(&active)
		if err != nil || !active {
			writeError(c, http.StatusUnauthorized, "SESSION_REVOKED", "登录会话已失效")
			c.Abort()
			return
		}

		c.Set("user_id", userID)
		c.Set("session_id", claims.ID)
		c.Next()
	}
}

func (app *application) me(c *gin.Context) {
	userID := c.GetInt64("user_id")
	var currentUser user
	err := app.db.QueryRowContext(c.Request.Context(), `
		SELECT id, nickname, avatar_url FROM users WHERE id = $1
	`, userID).Scan(&currentUser.ID, &currentUser.Nickname, &currentUser.AvatarURL)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(c, http.StatusNotFound, "USER_NOT_FOUND", "用户不存在")
		return
	}
	if err != nil {
		writeError(c, http.StatusInternalServerError, "INTERNAL_ERROR", "读取用户失败")
		return
	}
	c.JSON(http.StatusOK, gin.H{"user": currentUser})
}

func randomToken(byteLength int) (string, error) {
	buffer := make([]byte, byteLength)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buffer), nil
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func writeError(c *gin.Context, status int, code, message string) {
	c.JSON(status, gin.H{
		"code":    code,
		"message": message,
	})
}
```

## 7. 配置并启动后端

先生成一个强随机 JWT 密钥：

```bash
openssl rand -base64 48
```

macOS、Linux 或 Git Bash：

```bash
export SERVER_ADDR=':8080'
export DATABASE_URL='postgres://wechat:wechat@127.0.0.1:5432/wechat_login?sslmode=disable'
export WECHAT_APP_ID='wx替换成你的小程序AppID'
export WECHAT_APP_SECRET='替换成你的小程序AppSecret'
export JWT_SECRET='替换成至少32字符的随机字符串'
export ACCESS_TOKEN_TTL='15m'
export REFRESH_TOKEN_TTL='720h'

go run ./cmd/server
```

Windows PowerShell：

```powershell
$env:SERVER_ADDR = ':8080'
$env:DATABASE_URL = 'postgres://wechat:wechat@127.0.0.1:5432/wechat_login?sslmode=disable'
$env:WECHAT_APP_ID = 'wx替换成你的小程序AppID'
$env:WECHAT_APP_SECRET = '替换成你的小程序AppSecret'
$env:JWT_SECRET = '替换成至少32字符的随机字符串'
$env:ACCESS_TOKEN_TTL = '15m'
$env:REFRESH_TOKEN_TTL = '720h'

go run ./cmd/server
```

检查服务：

```bash
curl http://127.0.0.1:8080/healthz
```

预期响应：

```json
{"status":"ok"}
```

## 8. 编写小程序登录封装

下面把登录、续期、鉴权请求和退出统一放在 `app.js`。页面只需调用 `getApp().request()`，不需要重复处理 Token。

```javascript
const BASE_URL = 'http://127.0.0.1:8080'
const REFRESH_TOKEN_KEY = 'refresh_token'

function wxRequest(options) {
  return new Promise((resolve, reject) => {
    wx.request({
      ...options,
      success: resolve,
      fail: reject,
    })
  })
}

function wxLogin() {
  return new Promise((resolve, reject) => {
    wx.login({
      success: resolve,
      fail: reject,
    })
  })
}

App({
  globalData: {
    accessToken: '',
    refreshToken: '',
    user: null,
  },

  loginPromise: null,
  refreshPromise: null,

  onLaunch() {
    this.ensureLogin().catch((error) => {
      console.error('恢复登录失败', error)
    })
  },

  async ensureLogin() {
    if (this.globalData.accessToken) {
      return
    }
    if (this.loginPromise) {
      return this.loginPromise
    }

    this.loginPromise = (async () => {
      const savedRefreshToken = wx.getStorageSync(REFRESH_TOKEN_KEY)
      if (savedRefreshToken) {
        try {
          await this.refreshSession(savedRefreshToken)
          return
        } catch (error) {
          this.clearSession()
        }
      }
      await this.weChatLogin()
    })()

    try {
      await this.loginPromise
    } finally {
      this.loginPromise = null
    }
  },

  async weChatLogin() {
    const loginResult = await wxLogin()
    if (!loginResult.code) {
      throw new Error('wx.login 未返回 code')
    }

    const response = await wxRequest({
      url: `${BASE_URL}/api/v1/auth/wechat/login`,
      method: 'POST',
      data: {
        code: loginResult.code,
      },
    })
    if (response.statusCode !== 200) {
      throw new Error(response.data?.message || '微信登录失败')
    }
    this.saveSession(response.data)
  },

  async refreshSession(refreshToken = this.globalData.refreshToken) {
    if (this.refreshPromise) {
      return this.refreshPromise
    }
    if (!refreshToken) {
      throw new Error('缺少 refresh_token')
    }

    this.refreshPromise = (async () => {
      const response = await wxRequest({
        url: `${BASE_URL}/api/v1/auth/refresh`,
        method: 'POST',
        data: {
          refresh_token: refreshToken,
        },
      })
      if (response.statusCode !== 200) {
        throw new Error(response.data?.message || '刷新登录状态失败')
      }
      this.saveSession(response.data)
    })()

    try {
      await this.refreshPromise
    } finally {
      this.refreshPromise = null
    }
  },

  async request(options, retried = false) {
    await this.ensureLogin()
    const response = await wxRequest({
      ...options,
      url: `${BASE_URL}${options.url}`,
      header: {
        ...options.header,
        Authorization: `Bearer ${this.globalData.accessToken}`,
      },
    })

    if (response.statusCode === 401 && !retried) {
      try {
        await this.refreshSession()
      } catch (error) {
        this.clearSession()
        await this.ensureLogin()
      }
      return this.request(options, true)
    }
    if (response.statusCode < 200 || response.statusCode >= 300) {
      throw new Error(response.data?.message || `请求失败：${response.statusCode}`)
    }
    return response.data
  },

  async logout() {
    const refreshToken = this.globalData.refreshToken
    try {
      if (refreshToken) {
        await wxRequest({
          url: `${BASE_URL}/api/v1/auth/logout`,
          method: 'POST',
          data: {
            refresh_token: refreshToken,
          },
        })
      }
    } finally {
      this.clearSession()
    }
  },

  saveSession(data) {
    this.globalData.accessToken = data.access_token
    this.globalData.refreshToken = data.refresh_token
    if (data.user) {
      this.globalData.user = data.user
    }
    wx.setStorageSync(REFRESH_TOKEN_KEY, data.refresh_token)
  },

  clearSession() {
    this.globalData.accessToken = ''
    this.globalData.refreshToken = ''
    this.globalData.user = null
    wx.removeStorageSync(REFRESH_TOKEN_KEY)
  },
})
```

这个实现有几个关键点：

- `access_token` 只放内存，降低长期泄露风险；
- `refresh_token` 放本地存储，用于下次启动恢复登录；
- `refreshPromise` 避免多个并发 401 同时刷新；
- 刷新失败时重新执行 `wx.login()`；
- 401 后最多重试一次，避免无限循环；
- 每次刷新都会保存服务端返回的新 `refresh_token`。

## 9. 页面调用受保护接口

例如在页面的 `onLoad` 中读取当前用户：

```javascript
Page({
  data: {
    user: null,
    loading: true,
  },

  async onLoad() {
    try {
      const result = await getApp().request({
        url: '/api/v1/me',
        method: 'GET',
      })
      this.setData({
        user: result.user,
      })
    } catch (error) {
      wx.showToast({
        title: error.message || '加载失败',
        icon: 'none',
      })
    } finally {
      this.setData({ loading: false })
    }
  },
})
```

退出登录：

```javascript
await getApp().logout()
wx.reLaunch({ url: '/pages/index/index' })
```

## 10. 调试完整流程

### 10.1 开发者工具配置

1. 用微信开发者工具打开小程序；
2. 确认项目 AppID 与后端 `WECHAT_APP_ID` 一致；
3. 本地开发阶段，在“详情 → 本地设置”中临时勾选“不校验合法域名”；
4. 编译小程序；
5. 在 Network 面板观察登录请求。

正常情况下会出现：

```text
POST /api/v1/auth/wechat/login  200
GET  /api/v1/me                 200
```

登录成功响应示例：

```json
{
  "user": {
    "id": 1,
    "nickname": "微信用户",
    "avatar_url": ""
  },
  "access_token": "eyJ...",
  "refresh_token": "随机字符串",
  "token_type": "Bearer",
  "expires_in": 900
}
```

### 10.2 为什么不能提前用 curl 测试登录

登录接口需要一个真实且未使用的 `wx.login()` code。这个 code：

- 有效期很短；
- 只能使用一次；
- 与当前小程序 AppID 对应。

因此，最可靠的测试方式是让小程序现场调用 `wx.login()`，立即把 code 发送给 Gin 后端。不要复制旧 code 反复请求。

### 10.3 用 curl 测试鉴权接口

拿到登录响应中的 `access_token` 后，可以测试：

```bash
curl http://127.0.0.1:8080/api/v1/me \
  -H 'Authorization: Bearer 替换成access_token'
```

测试刷新：

```bash
curl -X POST http://127.0.0.1:8080/api/v1/auth/refresh \
  -H 'Content-Type: application/json' \
  -d '{"refresh_token":"替换成refresh_token"}'
```

注意：刷新成功后旧 `refresh_token` 立即失效，后续必须使用响应中的新值。

测试退出：

```bash
curl -i -X POST http://127.0.0.1:8080/api/v1/auth/logout \
  -H 'Content-Type: application/json' \
  -d '{"refresh_token":"替换成最新refresh_token"}'
```

退出后，数据库会话被撤销；本教程的鉴权中间件也会检查会话状态，因此原来的 `access_token` 会立即失效。

## 11. 用户昵称和头像怎么获取

`wx.login()` 和 `jscode2session` 不会返回昵称、头像、手机号等资料。登录只解决“这个微信用户是谁”。

如果需要昵称和头像，应在界面上让用户主动填写或选择：

- 昵称：使用昵称输入能力；
- 头像：使用 `button open-type="chooseAvatar"`；
- 手机号：使用 `button open-type="getPhoneNumber"`，并在后端调用相应微信接口处理动态 code。

不要把“获取用户资料”和“建立登录态”强行绑定。用户即使不提供头像和昵称，也应该能够完成基础登录。

本教程的登录接口接受可选的 `nickname` 和 `avatar_url`，只是为了演示资料写入；生产环境应增加长度、格式、内容安全和图片来源校验。

## 12. 核心安全设计说明

### 12.1 AppSecret 只在后端

小程序代码会被下载到用户设备，不能保存秘密。由 Gin 后端请求 `jscode2session`，是为了保护 `AppSecret`。

### 12.2 不把微信 code 当登录态

code 短期且一次性，只用于交换 `openid`。业务接口应该验证自己的 `access_token`，不能长期保存或重复使用 code。

### 12.3 refresh token 只保存哈希

数据库泄露时，攻击者无法直接使用哈希值调用刷新接口。服务端收到明文 refresh token 后计算 SHA-256，再与数据库记录匹配。

### 12.4 refresh token 必须轮换

每次刷新都生成新 token 并覆盖旧哈希，旧 token 立即失效。这能缩短 refresh token 被复制后的可利用窗口。

### 12.5 JWT 限定算法和签发者

解析 JWT 时显式限制 `HS256`，并校验 `iss`，避免接受非预期算法或其他系统签发的 Token。

### 12.6 日志必须脱敏

不要记录：

- 微信 code；
- AppSecret；
- session_key；
- openid 明文；
- access token；
- refresh token；
- 完整 `Authorization` 请求头。

## 13. 常见错误排查

### `40029 invalid code`

常见原因：

- code 已使用；
- code 已过期；
- code 属于另一个 AppID；
- 小程序重新编译后仍发送了旧 code。

处理方式：重新调用 `wx.login()`，立即提交新的 code。

### `40163 code been used`

同一个 code 被使用了第二次。检查前端是否重复提交登录请求，并用 `loginPromise` 合并并发登录。

### `40125 invalid appsecret`

后端配置的 AppSecret 不正确，或公众平台已经重置过密钥。更新环境变量并重启服务。

### 小程序请求不到 `127.0.0.1`

- 开发者工具中的 `127.0.0.1` 指向电脑本机；
- 真机中的 `127.0.0.1` 指向手机自己。

真机调试应使用手机可访问的局域网 IP，或部署到 HTTPS 公网域名。

### 合法域名校验失败

开发者工具中可以临时关闭校验，但正式发布前必须：

1. 使用已备案的 HTTPS 域名；
2. 在微信公众平台配置 `request` 合法域名；
3. 证书链完整且有效；
4. 重新启用域名校验并进行真机测试。

### 后端返回 `WECHAT_LOGIN_FAILED`

检查：

- `WECHAT_APP_ID` 与小程序项目 AppID 是否一致；
- `WECHAT_APP_SECRET` 是否正确；
- 服务器是否可以访问 `api.weixin.qq.com`；
- 系统时间是否正常；
- 后端日志中的微信 `errcode`，但不要打印 code 或密钥。

### 登录成功后 `/me` 返回 401

检查请求头格式：

```http
Authorization: Bearer eyJ...
```

还要确认：

- access token 是否过期；
- JWT 密钥是否在服务重启后发生变化；
- 对应 `auth_sessions` 是否被撤销或过期；
- 小程序是否保存了刷新后返回的新 refresh token。

## 14. 上线前必须补充的能力

教程已经实现完整登录闭环，但生产系统还应根据风险补充：

- 登录和刷新接口限流；
- 用户、会话和登录操作审计；
- 设备标识与会话管理页面；
- refresh token 重放检测与 token family；
- 密钥轮换和 `kid`；
- 多实例部署时统一数据库和密钥；
- PostgreSQL TLS、备份和 migration 管理；
- 统一请求 ID、结构化日志和监控告警；
- 昵称、头像 URL 和上传内容校验；
- 用户停用、封禁和注销流程；
- 定时清理过期、撤销的会话；
- 反向代理下的真实客户端 IP 和可信代理配置；
- 隐私政策、用户协议和个人信息最小化处理。

如果每个受保护请求都查询会话表带来性能压力，可以采用短期 access token + Redis 会话缓存，或者接受“退出后 access token 最长再存活 15 分钟”的策略。两者是安全性、复杂度和性能之间的权衡。

## 15. 上线检查清单

- [ ] AppSecret 只存在于服务端密钥管理或私有环境变量；
- [ ] JWT 密钥足够随机，且至少 32 字节；
- [ ] 生产环境使用 HTTPS；
- [ ] 微信公众平台已配置合法域名；
- [ ] 数据库连接启用 TLS；
- [ ] code、openid、session_key 和 Token 不进入日志；
- [ ] access token 有效期较短；
- [ ] refresh token 使用安全随机数生成；
- [ ] 数据库只保存 refresh token 哈希；
- [ ] refresh token 每次使用后轮换；
- [ ] 退出登录会撤销服务端会话；
- [ ] 登录和刷新接口有限流；
- [ ] 错误响应不暴露微信密钥或内部堆栈；
- [ ] 真机完成登录、刷新、退出和重新登录测试。

## 16. 最终调用关系

完成后，可以用下面这条主线检查自己是否真正理解了实现：

```text
wx.login()
  → 一次性 code
  → POST /api/v1/auth/wechat/login
  → Gin 请求 jscode2session
  → 获得 openid
  → PostgreSQL 查找或创建 user
  → 创建 auth_session
  → 返回 access_token + refresh_token
  → Authorization: Bearer access_token
  → Gin JWT 中间件校验
  → 读取当前业务用户
```

至此，一个不依赖任何既有项目、基于 Gin 的微信小程序登录闭环就完成了。
