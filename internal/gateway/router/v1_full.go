package router

import (
	"net/http"

	"github.com/gin-gonic/gin"

	account "knowledge-base/internal/account/handler"
	chat "knowledge-base/internal/chat/handler"
	document "knowledge-base/internal/document/handler"
	indexing "knowledge-base/internal/indexing/handler"
	library "knowledge-base/internal/library/handler"
	"knowledge-base/internal/platform/httpx"
	usage "knowledge-base/internal/usage/handler"
)

type V1Handlers struct {
	Verifier httpx.TokenVerifier
	Account  *account.Handler
	Library  *library.Handler
	Document *document.Handler
	Indexing *indexing.Handler
	Chat     *chat.Handler
	Usage    *usage.Handler
}

func registerV1(engine *gin.Engine, handlers V1Handlers) {
	v1 := engine.Group("/api/v1")
	// 小程序启动时先探测该接口，避免把只支持旧 /api 路由的进程误判为在线。
	v1.GET("/status", func(context *gin.Context) {
		context.JSON(http.StatusOK, gin.H{
			"version":      "v1",
			"capabilities": []string{"wechat_login", "libraries", "documents", "index_jobs", "chat_stream", "usage", "audit_logs", "miniprogram"},
		})
	})
	registerPublicAuth(v1, handlers.Account)
	authenticated := v1.Group("")
	authenticated.Use(httpx.Auth(handlers.Verifier))
	registerAccountRoutes(authenticated, handlers.Account)
	registerLibraryRoutes(authenticated, handlers.Library)
	registerDocumentRoutes(authenticated, handlers.Document)
	registerIndexingRoutes(authenticated, handlers.Indexing)
	registerChatRoutes(authenticated, handlers.Chat)
	registerUsageRoutes(authenticated, handlers.Usage)
}

func registerPublicAuth(group *gin.RouterGroup, handler *account.Handler) {
	group.POST("/auth/wechat/login", route(handler != nil, func(context *gin.Context) { handler.WeChatLogin(context) }))
	group.POST("/auth/refresh", route(handler != nil, func(context *gin.Context) { handler.Refresh(context) }))
	group.POST("/auth/web/tickets", route(handler != nil, func(context *gin.Context) { handler.CreateWebTicket(context) }))
	group.GET("/auth/web/tickets/:id", route(handler != nil, func(context *gin.Context) { handler.PollWebTicket(context) }))
}

func registerAccountRoutes(group *gin.RouterGroup, handler *account.Handler) {
	group.POST("/auth/logout", route(handler != nil, func(context *gin.Context) { handler.Logout(context) }))
	group.POST("/auth/web/tickets/:id/confirm", route(handler != nil, func(context *gin.Context) { handler.ConfirmWebTicket(context) }))
	group.GET("/me", route(handler != nil, func(context *gin.Context) { handler.Me(context) }))
	group.GET("/me/sessions", route(handler != nil, func(context *gin.Context) { handler.Sessions(context) }))
	group.DELETE("/me/sessions/:id", route(handler != nil, func(context *gin.Context) { handler.RevokeSession(context) }))
	group.DELETE("/me/sessions", route(handler != nil, func(context *gin.Context) { handler.RevokeOtherSessions(context) }))
}

func registerLibraryRoutes(group *gin.RouterGroup, handler *library.Handler) {
	group.GET("/libraries", route(handler != nil, func(context *gin.Context) { handler.List(context) }))
	group.POST("/libraries", route(handler != nil, func(context *gin.Context) { handler.Create(context) }))
	group.GET("/libraries/public", route(handler != nil, func(context *gin.Context) { handler.List(context) }))
	group.GET("/libraries/:id", route(handler != nil, func(context *gin.Context) { handler.Get(context) }))
	group.PATCH("/libraries/:id", route(handler != nil, func(context *gin.Context) { handler.Update(context) }))
	group.DELETE("/libraries/:id", route(handler != nil, func(context *gin.Context) { handler.Delete(context) }))
	group.GET("/libraries/:id/retrieval-settings", route(handler != nil, func(context *gin.Context) { handler.Settings(context) }))
	group.PATCH("/libraries/:id/retrieval-settings", route(handler != nil, func(context *gin.Context) { handler.UpdateSettings(context) }))
	group.POST("/libraries/:id/reindex", route(handler != nil, func(context *gin.Context) { handler.Reindex(context) }))
}

func registerDocumentRoutes(group *gin.RouterGroup, handler *document.Handler) {
	group.GET("/documents", route(handler != nil, func(context *gin.Context) { handler.List(context) }))
	group.POST("/documents", route(handler != nil, func(context *gin.Context) { handler.Upload(context) }))
	group.GET("/documents/:id", route(handler != nil, func(context *gin.Context) { handler.Get(context) }))
	group.PATCH("/documents/:id", route(handler != nil, func(context *gin.Context) { handler.Update(context) }))
	group.DELETE("/documents/:id", route(handler != nil, func(context *gin.Context) { handler.Delete(context) }))
	group.GET("/documents/:id/content", route(handler != nil, func(context *gin.Context) { handler.Content(context) }))
	group.PATCH("/documents/:id/content", route(handler != nil, func(context *gin.Context) { handler.EditContent(context) }))
	group.POST("/documents/:id/reindex", route(handler != nil, func(context *gin.Context) { handler.Reindex(context) }))
}

func registerIndexingRoutes(group *gin.RouterGroup, handler *indexing.Handler) {
	group.GET("/index-jobs/:id", route(handler != nil, func(context *gin.Context) { handler.Get(context) }))
	group.POST("/index-jobs/:id/retry", route(handler != nil, func(context *gin.Context) { handler.Retry(context) }))
}

func registerChatRoutes(group *gin.RouterGroup, handler *chat.Handler) {
	group.GET("/chat/sessions", route(handler != nil, func(context *gin.Context) { handler.ListSessions(context) }))
	group.POST("/chat/sessions", route(handler != nil, func(context *gin.Context) { handler.CreateSession(context) }))
	group.GET("/chat/sessions/:id", route(handler != nil, func(context *gin.Context) { handler.History(context) }))
	group.DELETE("/chat/sessions/:id", route(handler != nil, func(context *gin.Context) { handler.DeleteSession(context) }))
	group.POST("/chat/sessions/:id/messages/stream", route(handler != nil, func(context *gin.Context) { handler.Stream(context) }))
}

func registerUsageRoutes(group *gin.RouterGroup, handler *usage.Handler) {
	group.GET("/usage/summary", route(handler != nil, func(context *gin.Context) { handler.Summary(context) }))
	group.GET("/usage/records", route(handler != nil, func(context *gin.Context) { handler.Records(context) }))
	group.GET("/audit-logs", route(handler != nil, func(context *gin.Context) { handler.AuditLogs(context) }))
}

func route(available bool, handler gin.HandlerFunc) gin.HandlerFunc {
	if available {
		return handler
	}
	return func(context *gin.Context) {
		httpx.WriteError(context, &httpx.Error{Status: http.StatusServiceUnavailable, Code: "SERVICE_UNAVAILABLE", Message: "服务尚未初始化"})
	}
}
