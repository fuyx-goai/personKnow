import { createApp } from 'vue'
import { createRouter, createWebHashHistory } from 'vue-router'
import App from './App.vue'
import AskView from './views/AskView.vue'
import DocumentsView from './views/DocumentsView.vue'
import LibrariesView from './views/LibrariesView.vue'
import ProfileView from './views/ProfileView.vue'
import './styles/base.css'
import './styles/library.css'

const routes = [
  { path: '/', redirect: '/ask' },
  { path: '/ask', component: AskView, meta: { title: '问答对答' } },
  { path: '/libraries', component: LibrariesView, meta: { title: '知识库体系' } },
  { path: '/documents', component: DocumentsView, meta: { title: '原资料操作' } },
  { path: '/profile', component: ProfileView, meta: { title: '我的空间' } },
  { path: '/:pathMatch(.*)*', redirect: '/ask' },
]

const router = createRouter({ history: createWebHashHistory(), routes, scrollBehavior: () => ({ top: 0 }) })
createApp(App).use(router).mount('#app')
