# 微信小程序版本实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 新增遵循 Figma 原型的微信原生小程序，并接入当前 Go 知识库 API。

**Architecture:** 小程序位于独立 `miniprogram/` 目录。页面通过 `services/api.js` 访问网关，纯函数工具负责 SSE、聚合和格式化，公共组件负责顶部栏和底部导航，四个页面只维护自身状态与交互。

**Tech Stack:** 微信原生小程序、CommonJS JavaScript、WXML、WXSS、Node `node:test`。

## Global Constraints

- 不修改现有后端 API 契约，不新增 npm 依赖。
- 不使用 Mock 数据冒充服务端成功结果。
- 所有 JavaScript、WXML、WXSS 单文件不超过 300 行。
- 删除操作必须二次确认，网络错误必须可见。
- 小程序真机请求需由使用者配置 HTTPS 合法域名。

---

### Task 1: 小程序基础骨架

**Files:**
- Create: `miniprogram/project.config.json`
- Create: `miniprogram/app.js`
- Create: `miniprogram/app.json`
- Create: `miniprogram/app.wxss`
- Create: `miniprogram/sitemap.json`

- [x] 建立四页面路由、全局状态、设计令牌和开发工具配置。
- [x] 加入 API 地址本地存储与全局刷新入口。

### Task 2: 纯函数 TDD

**Files:**
- Create: `miniprogram/tests/sse.test.js`
- Create: `miniprogram/tests/library.test.js`
- Create: `miniprogram/utils/sse.js`
- Create: `miniprogram/utils/library.js`
- Create: `miniprogram/utils/format.js`

- [x] 先写 SSE 分块、UTF-8 边界、引用事件测试并确认失败。
- [x] 实现最小 SSE 解码与解析逻辑并确认测试通过。
- [x] 先写来源聚合、搜索与格式化测试并确认失败。
- [x] 实现最小聚合与格式化逻辑并确认测试通过。

### Task 3: API 与公共组件

**Files:**
- Create: `miniprogram/services/api.js`
- Create: `miniprogram/components/app-header/*`
- Create: `miniprogram/components/bottom-nav/*`
- Create: `miniprogram/components/state-view/*`

- [x] 封装健康、配置、统计、片段、摄入、删除和流式问答。
- [x] 实现安全区顶部栏、四栏底部导航与通用状态组件。

### Task 4: 四个业务页面

**Files:**
- Create: `miniprogram/pages/chat/*`
- Create: `miniprogram/pages/libraries/*`
- Create: `miniprogram/pages/sources/*`
- Create: `miniprogram/pages/profile/*`

- [x] 实现流式问答、引用卡片和知识库选择视觉。
- [x] 实现统计、来源分组、搜索和问答跳转。
- [x] 实现资料搜索、服务端路径摄入和按来源删除。
- [x] 实现服务信息、用量统计与 API 地址设置。

### Task 5: 文档与验证

**Files:**
- Create: `miniprogram/README.md`

- [x] 记录微信开发者工具导入、域名配置和功能边界。
- [x] 运行 Node 测试、JS 语法、JSON 解析、文件行数与 Go 测试。
