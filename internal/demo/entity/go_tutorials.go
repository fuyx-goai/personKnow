package entity

const goWebsiteRawBase = "https://raw.githubusercontent.com/golang/website/master/_content/doc/tutorial/"
const goWebsitePageBase = "https://github.com/golang/website/blob/master/_content/doc/tutorial/"

// GoTutorialSources 是经许可证审核的 Go 官方教程白名单，不接受运行时传入任意 URL。
func GoTutorialSources() []RemoteDocumentSeed {
	return []RemoteDocumentSeed{
		goTutorial("database-access", "Go 访问关系型数据库.md", "database-access.md", "database/sql 连接、查询与参数化 SQL 官方教程。", []string{"Go", "数据库", "官方教程"}),
		goTutorial("fuzz", "Go Fuzzing 模糊测试.md", "fuzz.md", "使用 Go fuzzing 发现边界输入与崩溃条件。", []string{"Go", "Fuzzing", "测试"}),
		goTutorial("generics", "Go 泛型入门.md", "generics.md", "用类型参数和约束编写可复用的 Go 函数。", []string{"Go", "泛型", "官方教程"}),
		goTutorial("govulncheck", "Go 依赖漏洞检查.md", "govulncheck.md", "使用 govulncheck 定位代码真正可达的已知漏洞。", []string{"Go", "安全", "govulncheck"}),
		goTutorial("govulncheck-ide", "在 IDE 中检查 Go 漏洞.md", "govulncheck-ide.md", "将 Go 漏洞检查接入编辑器开发流程。", []string{"Go", "IDE", "安全"}),
		goTutorial("json", "Go JSON 处理教程.md", "json.md", "使用 encoding/json 处理结构化数据的官方教程。", []string{"Go", "JSON", "标准库"}),
		goTutorial("web-service-gin", "使用 Gin 开发 Go Web API.md", "web-service-gin.md", "使用 Gin 设计 REST API 路由、参数与响应。", []string{"Go", "Gin", "Web API"}),
		goTutorial("workspaces", "Go Workspace 多模块开发.md", "workspaces.md", "使用 go work 在本地同时开发多个 Go module。", []string{"Go", "Workspace", "Module"}),
	}
}

func goTutorial(key, name, path, summary string, tags []string) RemoteDocumentSeed {
	return RemoteDocumentSeed{
		DocumentSeed: DocumentSeed{Key: "github-go-" + key, LibraryKey: "go", Name: name, Summary: summary, Tags: tags},
		RawURL:       goWebsiteRawBase + path, SourceURL: goWebsitePageBase + path,
		Repository: "golang/website", License: "BSD-3-Clause",
	}
}
