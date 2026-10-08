// v1.go —— 网关层：业务路由（按接口版本分组）
//
// bw-cli 的约定是 /api/v1；本项目对外接口沿用 /api（避免破坏已有调用方）。
// 将来若要升级到 /api/v1，只改下面这一行 group 前缀即可，handler 一行都不用动：
//
//	api := r.Group("/api/v1")
package router

import (
	"github.com/gin-gonic/gin"

	"knowledge-base/internal/gateway/handler"
)

// Endpoints 当前对外提供的接口清单（根路由与文档共用，避免两处不同步）
var Endpoints = []string{
	"GET  /api/health",
	"GET  /api/config",
	"GET  /api/stats",
	"GET  /api/chunks",
	"DELETE /api/chunks",
	"POST /api/ingest",
	"POST /api/chat",
	"POST /api/chat/stream",
}

// registerAPI 注册 /api 分组下的全部路由
func registerAPI(r *gin.Engine, h *handler.Handler) {
	api := r.Group("/api")
	api.GET("/health", h.Health)
	api.GET("/config", h.Config)
	api.GET("/stats", h.Stats)
	api.GET("/chunks", h.Chunks)          // 列出片段（藏书页）
	api.DELETE("/chunks", h.DeleteChunks) // 删除片段 / 清空整库
	api.POST("/ingest", h.Ingest)
	api.POST("/chat", h.Chat)
	api.POST("/chat/stream", h.ChatStream)
}
