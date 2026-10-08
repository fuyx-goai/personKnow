// router.go —— 网关层：路由装配（中间件 + 前端页面 + 版本分流）
//
// 为什么能这么轻松地把 RAG 套成 Web 接口？因为 service 层已经把它封装成
// "吃一个问题、吐一个答案"的用例方法，网关只需要做参数绑定与序列化。
//
// 路由拆成两个文件，与 bw-cli 脚手架保持一致：
//
//	router.go 全局中间件、前端静态资源、公共部分
//	v1.go     业务路由（按接口版本分组）
package router

import (
	"io/fs"
	"net/http"

	"github.com/gin-gonic/gin"

	"knowledge-base/internal/gateway/handler"
	"knowledge-base/internal/gateway/web"
)

// New 组装 Gin 引擎：挂上中间件、前端页面，再注册业务路由
func New(h *handler.Handler) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Logger(), gin.Recovery()) // 访问日志 + panic 兜底，避免一个请求崩掉整个服务

	registerWeb(r)
	registerAPI(r, h)
	return r
}

// registerWeb 挂载前端页面
//
// 前端是 Vue 3 + Vite 构建出的单页应用，产物用 go:embed 编译进了二进制
// （见 internal/gateway/web），所以不依赖运行目录，二进制拷到哪都能跑。
// 前后端同源，也就不用配 CORS 了。
//
// 路由用 hash 模式，因此这里只需要托住 "/" 和静态资源，
// 刷新任意页面都不会 404，不用做 history fallback。
func registerWeb(r *gin.Engine) {
	// Vite 把 JS/CSS 产在 dist/assets 下，这里把该目录挂到 /assets
	if assets, err := fs.Sub(web.Files, "dist/assets"); err == nil {
		r.StaticFS("/assets", http.FS(assets))
	}

	// 首页：知识库界面
	r.GET("/", func(c *gin.Context) {
		page, err := fs.ReadFile(web.Files, "dist/index.html")
		if err != nil {
			c.String(http.StatusInternalServerError,
				"前端资源缺失（%v）。请在 web/ 目录执行 npm run build，或直接 make web", err)
			return
		}
		c.Data(http.StatusOK, "text/html; charset=utf-8", page)
	})

	// 接口清单：方便用 curl 一眼看清有什么能力
	r.GET("/api", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"name": "Eino 个人知识库",
			"web":  "浏览器打开服务地址即为知识库界面",
			"api":  Endpoints,
		})
	})
}
