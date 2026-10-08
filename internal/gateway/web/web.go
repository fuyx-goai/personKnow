// Package web 把 Vue 前端（知识库界面）的构建产物编译进 Go 二进制
//
// 为什么用 go:embed 而不是让 Gin 去读磁盘上的目录？
//
//	go:embed ：资源进二进制，拷到哪都能跑，不依赖"当前工作目录"
//	读磁盘   ：从别的目录启动（比如 systemd、Docker）就会 404
//
// 代价是改完前端得重新编译一次——而 go run / make run 本来就会重编，影响不大。
//
// 前端源码在仓库根的 web/ 目录（Vue 3 + Vite），构建产物输出到这里：
//
//	dist/index.html      单页应用入口（Vite 生成的哈希资源引用）
//	dist/assets/*.js     Vue 运行时与业务逻辑
//	dist/assets/*.css    样式
//
// 只有改前端时才需要跑 `make web`（等价于 cd web && npm run build）。
// 平时 `go build` / `go run` 完全不需要 Node —— dist 已经提交在仓库里了。
package web

import "embed"

// Files 前端构建产物，dist/ 是它的根目录
//
//go:embed all:dist
var Files embed.FS
