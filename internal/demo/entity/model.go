package entity

import "github.com/google/uuid"

// LibrarySeed 描述一组原型中可见的知识库演示数据。
type LibrarySeed struct {
	Key         string
	Name        string
	Category    string
	Description string
}

// DocumentSeed 描述知识库中的示例资料；内容会同时保存为原文件和内容版本。
type DocumentSeed struct {
	Key          string
	LibraryKey   string
	Name         string
	Summary      string
	Tags         []string
	Content      string
	ChunkCount   int
	IndexedToken int64
}

// RemoteDocumentSeed 描述允许从 GitHub 拉取的教学资料及其授权信息。
type RemoteDocumentSeed struct {
	DocumentSeed
	RawURL     string
	SourceURL  string
	Repository string
	License    string
}

// SessionSeed 描述历史问答卡片及其可溯源回答。
type SessionSeed struct {
	Key          string
	LibraryKey   string
	Title        string
	Question     string
	Answer       string
	DocumentKey  string
	SourceName   string
	Excerpt      string
	Location     string
	InputTokens  int64
	OutputTokens int64
}

// SeedData 返回与 Figma 原型信息架构一致的本地演示数据。
func SeedData() ([]LibrarySeed, []DocumentSeed, []SessionSeed) {
	libraries := []LibrarySeed{
		{Key: "go", Name: "Go 工程实践", Category: "专业知识", Description: "整理 Go 并发、服务架构与工程化实践，支持技术问答。"},
		{Key: "ai", Name: "AI 学习与实践", Category: "学习笔记", Description: "沉淀大模型、RAG、Embedding 与提示词实验记录。"},
		{Key: "reading", Name: "个人读书卡片", Category: "读书笔记", Description: "记录阅读摘录、方法论与可执行的行动清单。"},
	}
	documents := []DocumentSeed{
		{Key: "go-concurrency", LibraryKey: "go", Name: "Go 并发编程实践.md", Summary: "从 goroutine、channel 到并发安全，整理日常服务开发中的实践准则。", Tags: []string{"Go", "并发", "工程化"}, ChunkCount: 12, IndexedToken: 1850, Content: "# Go 并发编程实践\n\n## Goroutine 与生命周期\n\n启动 goroutine 后要明确退出条件，使用 context 传递取消信号，避免后台任务泄漏。\n\n## Channel 使用原则\n\n由发送方关闭 channel；多个消费者共享任务时优先使用明确的所有权和等待组。\n\n## 并发安全\n\n共享状态需要 mutex 或原子操作保护，优先通过消息传递降低锁的范围。\n"},
		{Key: "service-layout", LibraryKey: "go", Name: "服务端目录与分层规范.md", Summary: "记录 gateway、service、repo 分层边界，以及可测试的依赖注入方式。", Tags: []string{"架构", "分层", "Go"}, ChunkCount: 9, IndexedToken: 1420, Content: "# 服务端目录与分层规范\n\nHTTP handler 只负责协议绑定和响应序列化，业务规则放在 service，数据库和外部系统适配放在 repo。\n\ncmd 负责配置读取、依赖装配和生命周期管理，不承载业务规则。\n"},
		{Key: "rag-notes", LibraryKey: "ai", Name: "RAG 检索增强生成笔记.md", Summary: "总结切块、向量召回、重排和引用溯源组成的可信问答链路。", Tags: []string{"RAG", "Embedding", "检索"}, ChunkCount: 16, IndexedToken: 2680, Content: "# RAG 检索增强生成\n\nRAG 的核心链路是文档解析、语义切块、向量化、召回、重排和带引用生成。\n\n切块大小要结合资料结构调整；召回结果需要保留来源、版本和位置，回答才可以被复核。\n"},
		{Key: "prompt-playbook", LibraryKey: "ai", Name: "提示词工作手册.md", Summary: "整理角色、约束、输出格式和验收标准，让模型输出更稳定。", Tags: []string{"Prompt", "模型", "实践"}, ChunkCount: 8, IndexedToken: 1160, Content: "# 提示词工作手册\n\n先说明任务目标，再给出上下文、约束和输出格式；复杂任务拆成可验证的小步骤。\n\n对关键结果增加验收标准，避免只描述过程而没有可执行产出。\n"},
		{Key: "reading-cards", LibraryKey: "reading", Name: "长期主义读书卡片.md", Summary: "把阅读摘录转化为原则、案例和下一步行动，方便持续复盘。", Tags: []string{"阅读", "方法论", "复盘"}, ChunkCount: 7, IndexedToken: 980, Content: "# 长期主义读书卡片\n\n有价值的摘录需要连接到真实问题；每张卡片至少保留一个观点、一个案例和一个可执行行动。\n\n定期回顾卡片，删除无法指导行动的内容，让知识库保持轻量和可检索。\n"},
	}
	documents = append(documents, AIArticles()...)
	sessions := []SessionSeed{
		{Key: "rag-summary", LibraryKey: "ai", Title: "总结 RAG 检索的核心主题", Question: "总结当前知识库的核心主题", Answer: "当前资料主要围绕 RAG 的完整链路展开：先解析和切块，再完成向量召回与重排，最后生成带来源引用的回答。实践中还要关注版本管理、权限范围和可复核性。", DocumentKey: "rag-notes", SourceName: "RAG 检索增强生成笔记.md", Excerpt: "RAG 的核心链路是文档解析、语义切块、向量化、召回、重排和带引用生成。", Location: "第 1 节 · 核心链路", InputTokens: 42, OutputTokens: 128},
		{Key: "go-question", LibraryKey: "go", Title: "解释 Go 并发的关键原则", Question: "解释资料中的关键技术术语", Answer: "资料建议先明确 goroutine 的退出条件，再用 context 传递取消信号；共享状态需要 mutex 或原子操作保护，channel 则应遵循发送方关闭原则。", DocumentKey: "go-concurrency", SourceName: "Go 并发编程实践.md", Excerpt: "启动 goroutine 后要明确退出条件，使用 context 传递取消信号，避免后台任务泄漏。", Location: "第 1 节 · Goroutine 与生命周期", InputTokens: 36, OutputTokens: 96},
		{Key: "reading-action", LibraryKey: "reading", Title: "哪些内容值得继续整理？", Question: "有哪些内容值得继续整理？", Answer: "可以继续把读书摘录连接到真实项目：为每个观点补充一个案例和一个行动，并定期回顾、删除无法指导行动的卡片。", DocumentKey: "reading-cards", SourceName: "长期主义读书卡片.md", Excerpt: "每张卡片至少保留一个观点、一个案例和一个可执行行动。", Location: "第 1 节 · 卡片结构", InputTokens: 31, OutputTokens: 82},
	}
	return libraries, documents, sessions
}

// ID 为同一用户生成稳定的 UUID，确保演示命令可安全重复执行。
func ID(userID uuid.UUID, key string) uuid.UUID {
	return uuid.NewSHA1(uuid.NameSpaceURL, []byte("personknow-demo:"+userID.String()+":"+key))
}

type Report struct {
	UserID       uuid.UUID `json:"user_id"`
	Libraries    int       `json:"libraries"`
	Documents    int       `json:"documents"`
	Sessions     int       `json:"sessions"`
	UsageRecords int       `json:"usage_records"`
}
