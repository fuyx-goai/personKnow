// main.js —— 应用入口：装配路由、挂载根组件
//
// 路由用 hash 模式（URL 形如 /#/library）：
//   · 不需要 Go 那边配置任何 history fallback，刷新页面也不会 404
//   · 个人自用的界面，URL 带个 # 完全不影响观感
import { createApp } from 'vue'
import { createRouter, createWebHashHistory } from 'vue-router'

import App from './App.vue'
import OverviewView from './views/OverviewView.vue'
import IngestView from './views/IngestView.vue'
import AskView from './views/AskView.vue'
import LibraryView from './views/LibraryView.vue'
import './styles/base.css'

const routes = [
  { path: '/', redirect: '/ask' },
  {
    path: '/overview',
    name: 'overview',
    component: OverviewView,
    meta: { title: '总览', note: '库里有几段知识、都是从哪来的' },
  },
  {
    path: '/ingest',
    name: 'ingest',
    component: IngestView,
    meta: { title: '摄入', note: '把笔记切分、向量化，收进库里' },
  },
  {
    path: '/ask',
    name: 'ask',
    component: AskView,
    meta: { title: '问答', note: '先检索、再作答，答案都带出处' },
  },
  {
    path: '/library',
    name: 'library',
    component: LibraryView,
    meta: { title: '藏书', note: '逐段翻阅已入库的内容，可按来源清理' },
  },
]

const router = createRouter({
  history: createWebHashHistory(),
  routes,
  scrollBehavior: () => ({ top: 0 }),
})

createApp(App).use(router).mount('#app')
