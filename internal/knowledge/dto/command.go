// command.go —— DTO 层：业务用例的"入参命令"
//
// 为什么不让 service 直接收一堆散参数？
//  1. 用例入参一旦变化（比如问答要加 topK、摄入要加并发数），只改命令结构体，
//     service 方法签名不动，调用方也不会连锁改动；
//  2. 命令是"业务语言"，与 HTTP 的请求体解耦 ——
//     gateway 负责把 request 翻译成 command，service 不必知道 HTTP 的存在。
package dto

// AskCommand 问答用例入参命令
type AskCommand struct {
	// Question 用户问题（已由调用方做过非空校验与去空格）
	Question string
}

// IngestCommand 摄入用例入参命令
type IngestCommand struct {
	// Paths 待摄入的文件或目录列表，可以是多个
	Paths []string
}
