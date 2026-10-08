# 豆包式前端重构实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在保留个人知识库现有路由、接口与业务行为的前提下，将前端重构为接近豆包桌面端的浅色 AI 工作台。

**Architecture:** 保留 Vue Router、共享 store 与 API 层，只替换应用壳层和各页面的展示结构。全局设计令牌集中在 `base.css`，导航、顶栏与图标组件提供一致外壳，各业务页面继续独立维护自身布局。

**Tech Stack:** Vue 3、Vue Router、Vite、原生 CSS、内联 SVG。

## Global Constraints

- 不修改后端 API、路由路径、SSE 问答流程与数据字段。
- 不新增 npm 依赖，不依赖远程图标或图片资源。
- 桌面端重点还原豆包式侧栏、大留白、圆角输入框与轻量蓝色强调色。
- 移动端保留四个核心入口，主要内容不能被固定输入框遮挡。
- 尊重 `prefers-reduced-motion`，所有按钮保留可见 focus 状态。

---

### Task 1: 全局设计系统与应用骨架

**Files:**
- Modify: `web/src/styles/base.css`
- Modify: `web/src/App.vue`
- Modify: `web/index.html`

**Interfaces:**
- Consumes: 现有 `.btn`、`.input`、`.panel`、`.tile`、`.empty`、`.md` 公共类。
- Produces: 豆包式颜色、圆角、阴影、页面尺寸与响应式基础令牌。

- [x] **Step 1:** 将纸张、朱砂与书脊令牌替换为灰白底、白色卡片、蓝色主色和柔和边框。
- [x] **Step 2:** 将页面骨架改为固定宽侧栏与自适应主内容区，加入桌面和窄屏布局。
- [x] **Step 3:** 更新公共按钮、输入框、卡片、空态和 Markdown 样式，确保 focus-visible 清晰。
- [x] **Step 4:** 更新网页标题、描述和 favicon，使品牌与新界面一致。

### Task 2: 导航与公共组件

**Files:**
- Create: `web/src/components/AppIcon.vue`
- Modify: `web/src/components/SpineNav.vue`
- Modify: `web/src/components/Masthead.vue`
- Modify: `web/src/components/ToastHost.vue`
- Modify: `web/src/components/ConfirmDialog.vue`
- Modify: `web/src/components/ScoreBar.vue`

**Interfaces:**
- Consumes: `state`、`shelves`、当前路由 meta。
- Produces: 统一的线性图标、侧栏导航、最近知识来源、顶部状态栏和轻量反馈组件。

- [x] **Step 1:** 新增无依赖 SVG 图标组件，覆盖导航、搜索、发送、状态和常用操作。
- [x] **Step 2:** 重构侧栏为品牌区、功能入口、最近来源和底部用户状态区。
- [x] **Step 3:** 将顶部报头压缩为豆包式工具栏，保留当前页面标题与运行状态。
- [x] **Step 4:** 统一提示、确认框和相似度条的圆角、阴影与颜色。

### Task 3: 业务页面重构

**Files:**
- Modify: `web/src/views/AskView.vue`
- Modify: `web/src/views/OverviewView.vue`
- Modify: `web/src/views/IngestView.vue`
- Modify: `web/src/views/LibraryView.vue`
- Modify: `web/src/components/ChunkCard.vue`

**Interfaces:**
- Consumes: 原有 `api`、`store`、路由和格式化工具。
- Produces: 豆包式问答首页、状态总览、知识摄入和资料库管理界面。

- [x] **Step 1:** 将问答空态改为居中欢迎区、推荐问题和底部悬浮输入框。
- [x] **Step 2:** 将问答记录改为窄栏对话流，保留流式响应、停止、清空和引用展开。
- [x] **Step 3:** 将总览页改为欢迎标题、关键统计卡和来源分布面板。
- [x] **Step 4:** 将摄入页、资料库和知识片段卡统一为轻量圆角工作台风格。

### Task 4: 验证与视觉收尾

**Files:**
- Verify: `web/`

**Interfaces:**
- Consumes: 完成后的前端源码。
- Produces: 可构建、可浏览且在桌面与窄屏下结构稳定的前端产物。

- [x] **Step 1:** 运行 `npm run build`，要求退出码为 0。
- [x] **Step 2:** 启动 Vite 开发服务并在浏览器检查问答、总览、摄入和资料库路由。
- [x] **Step 3:** 检查桌面与窄屏截图，修复溢出、遮挡、对齐和视觉层级问题。
- [x] **Step 4:** 再次运行 `npm run build`，确认最终源码可构建。
