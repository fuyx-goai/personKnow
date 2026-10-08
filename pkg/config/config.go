// config.go —— 全项目配置中心（HTTP 服务 + 向量库 + 大模型）
//
// 位于 pkg/ 下，属于"与业务无关、可被复用"的基础包：
// 各层（repo / gateway）都只依赖它暴露的 Config 结构体，不各自读文件。
//
// 所有配置项都写在 configs/config.yaml 里，本文件只负责
// "读文件 → 补默认值 → 打印自检信息"，改配置无需改动任何 Go 代码。
//
// 配置文件路径规则：
//  1. 环境变量 CONFIG_FILE 指定的路径（相对路径会逐级向上查找，便于子目录里跑测试）；
//  2. 都没有时用 configs/config.yaml。
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// ==================== 常量与默认值 ====================

// 向量库类型常量（configs/config.yaml 里 vector_store 的取值）
const (
	StoreMem    = "mem"    // 内存向量库（默认，开箱即用，落盘 knowledge.json）
	StoreMilvus = "milvus" // Milvus 向量库（需先启动 Milvus 服务）
)

// 向量化接口形态常量（configs/config.yaml 里 llm.embed_api 的取值）
//
// 方舟上两种向量模型的接口不一样，填错了会直接报 404 / "does not support this api"：
const (
	EmbedAPIOpenAI        = "openai"         // 标准 OpenAI 兼容 /embeddings，配纯文本向量模型
	EmbedAPIArkMultimodal = "ark_multimodal" // 方舟多模态 /embeddings/multimodal，配多模态向量模型
)

// DefaultConfigFile 默认配置文件路径（相对项目根目录）
const DefaultConfigFile = "configs/config.yaml"

// 兜底默认值：config.yaml 里漏写或写空的项，用这里的值顶上
const (
	defaultHTTPAddr       = ":8080"
	defaultBaseURL        = "https://ark.cn-beijing.volces.com/api/v3"
	defaultChatModel      = "doubao-seed-1-6-250615"
	defaultEmbedModel     = "doubao-embedding-large-text-240915"
	defaultMilvusAddr     = "127.0.0.1:19530"
	defaultMilvusUser     = "root"
	defaultMilvusPassword = "Milvus"
	defaultMilvusDBName   = "default"
	defaultMilvusColl     = "eino_knowledge"
	defaultMilvusDim      = 2048
	defaultMilvusMetric   = "COSINE"
)

// ==================== 配置结构 ====================
// 结构体上的 yaml 标签，决定它对应 config.yaml 里的哪个字段。

// LLMConfig 大模型配置（对应 configs/config.yaml 的 llm 段）
type LLMConfig struct {
	APIKey     string `yaml:"api_key"`     // API Key
	BaseURL    string `yaml:"base_url"`    // 接口根地址（只到 /api/v3，别带具体接口路径）
	ChatModel  string `yaml:"chat_model"`  // 对话模型
	EmbedModel string `yaml:"embed_model"` // 向量化模型
	EmbedAPI   string `yaml:"embed_api"`   // 向量化接口形态：openai / ark_multimodal
}

// MilvusConfig Milvus 连接与集合配置（对应 config.yaml 的 milvus 段）
type MilvusConfig struct {
	Address    string `yaml:"address"`    // 服务地址
	Username   string `yaml:"username"`   // 账号
	Password   string `yaml:"password"`   // 密码
	DBName     string `yaml:"db_name"`    // 数据库名
	Collection string `yaml:"collection"` // 集合名
	Dim        int    `yaml:"dim"`        // 向量维度
	Metric     string `yaml:"metric"`     // 度量方式（COSINE / IP / L2）
}

// Config 服务总体配置（对应 config.yaml 顶层）
type Config struct {
	HTTPAddr    string       `yaml:"http_addr"`    // HTTP 监听地址
	VectorStore string       `yaml:"vector_store"` // 向量库类型：mem / milvus
	LLM         LLMConfig    `yaml:"llm"`          // 大模型参数
	Milvus      MilvusConfig `yaml:"milvus"`       // Milvus 参数（vector_store=milvus 时生效）
}

// ==================== 加载 ====================

// Load 加载配置：找到配置文件 → 解析 YAML → 补齐默认值
func Load() (Config, error) {
	path := os.Getenv("CONFIG_FILE")
	if path == "" {
		path = DefaultConfigFile
	}

	found, err := locate(path)
	if err != nil {
		return Config{}, err
	}

	raw, err := os.ReadFile(found)
	if err != nil {
		return Config{}, fmt.Errorf("读取配置文件 %s 失败: %w", found, err)
	}

	var cfg Config
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		return Config{}, fmt.Errorf("解析配置文件 %s 失败（多为 YAML 缩进问题）: %w", found, err)
	}
	cfg.applyDefaults()
	return cfg, nil
}

// applyDefaults 给未填写的项补上默认值，并统一大小写
func (c *Config) applyDefaults() {
	c.HTTPAddr = pick(c.HTTPAddr, defaultHTTPAddr)
	c.VectorStore = strings.ToLower(pick(c.VectorStore, StoreMem))

	c.LLM.APIKey = strings.TrimSpace(c.LLM.APIKey)
	c.LLM.BaseURL = pick(c.LLM.BaseURL, defaultBaseURL)
	c.LLM.ChatModel = pick(c.LLM.ChatModel, defaultChatModel)
	c.LLM.EmbedModel = pick(c.LLM.EmbedModel, defaultEmbedModel)
	c.LLM.EmbedAPI = strings.ToLower(pick(c.LLM.EmbedAPI, EmbedAPIOpenAI))

	c.Milvus.Address = pick(c.Milvus.Address, defaultMilvusAddr)
	c.Milvus.Username = pick(c.Milvus.Username, defaultMilvusUser)
	c.Milvus.Password = pick(c.Milvus.Password, defaultMilvusPassword)
	c.Milvus.DBName = pick(c.Milvus.DBName, defaultMilvusDBName)
	c.Milvus.Collection = pick(c.Milvus.Collection, defaultMilvusColl)
	c.Milvus.Metric = strings.ToUpper(pick(c.Milvus.Metric, defaultMilvusMetric))
	if c.Milvus.Dim <= 0 {
		c.Milvus.Dim = defaultMilvusDim
	}
}

// locate 定位配置文件，让"项目根目录启动"和"子目录里跑测试"都能找到同一个文件
func locate(path string) (string, error) {
	// 绝对路径：直接用，找不到就报错，不做逐级查找
	if filepath.IsAbs(path) {
		if st, err := os.Stat(path); err == nil && !st.IsDir() {
			return path, nil
		}
		return "", fmt.Errorf("配置文件不存在: %s", path)
	}

	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	// 从当前目录向上最多找 5 层，覆盖 go run ./cmd/gateway、go test ./cmd/gateway 等情况
	for i := 0; i < 5; i++ {
		p := filepath.Join(dir, path)
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir { // 已到根目录
			break
		}
		dir = parent
	}
	return "", fmt.Errorf("找不到配置文件 %s（请在 configs/ 下创建它，或用 CONFIG_FILE 指定路径）", path)
}

// pick 取值：写非空就用写的，写空则回退默认值
func pick(value, fallback string) string {
	if v := strings.TrimSpace(value); v != "" {
		return v
	}
	return fallback
}

// ==================== 打印自检信息 ====================

// Describe 生成"当前配置"摘要，服务启动时打印，方便确认配置是否生效
// API Key 与 Milvus 密码都会打码，不用担心泄露到日志里
func (c Config) Describe() string {
	store := c.VectorStore
	if store == StoreMilvus {
		store = fmt.Sprintf("milvus(%s, collection=%s, dim=%d, metric=%s)",
			c.Milvus.Address, c.Milvus.Collection, c.Milvus.Dim, c.Milvus.Metric)
	}
	return fmt.Sprintf("服务配置 | 监听=%s | 向量库=%s\n%s", c.HTTPAddr, store, c.LLM.Describe())
}

// Describe 生成一行"大模型配置"信息（API Key 打码）
func (l LLMConfig) Describe() string {
	masked := "(未配置)"
	if len(l.APIKey) >= 8 {
		masked = l.APIKey[:4] + "****" + l.APIKey[len(l.APIKey)-4:]
	} else if l.APIKey != "" {
		masked = "****"
	}
	return fmt.Sprintf("LLM 配置 | baseURL=%s | 对话模型=%s | 向量模型=%s(%s) | apiKey=%s",
		l.BaseURL, l.ChatModel, l.EmbedModel, l.EmbedAPI, masked)
}
