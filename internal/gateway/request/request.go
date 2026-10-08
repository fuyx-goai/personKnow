// request.go —— 网关层：HTTP 入参 DTO
//
// 这一层是"协议适配"的第一道关口：只认 HTTP 的 JSON 形状，
// 校验通过后翻译成 dto 层的业务命令，service 完全不感知 HTTP 的存在。
//
// 注意：出参（响应体）不放这里，而是用 dto 层的业务出参，
// 这样"接口返回什么"由业务决定，"接口收什么"由协议决定，各归其位。
package request

// Ingest 摄入请求：path 与 paths 二选一
type Ingest struct {
	Path  string   `json:"path"`  // 单个文件或目录
	Paths []string `json:"paths"` // 多个路径
}

// AllPaths 归一化：把单数 path 合并进 paths，得到最终要处理的路径列表
func (r Ingest) AllPaths() []string {
	paths := make([]string, 0, len(r.Paths)+1)
	paths = append(paths, r.Paths...)
	if r.Path != "" {
		paths = append(paths, r.Path)
	}
	return paths
}

// Chat 问答请求
type Chat struct {
	Question string `json:"question"` // 用户问题
}
