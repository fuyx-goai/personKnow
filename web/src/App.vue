<script setup>
import { computed, onMounted } from 'vue'
import { useRoute } from 'vue-router'
import LoginView from './views/LoginView.vue'
import ToastHost from './components/ToastHost.vue'
import { bootstrap, state } from './store'

const route = useRoute()
const nav = [
  { to: '/ask', icon: '↗', label: '问答对答' },
  { to: '/libraries', icon: '⌘', label: '知识库体系' },
  { to: '/documents', icon: '▤', label: '原资料操作' },
  { to: '/profile', icon: '●', label: '我的空间' },
]
const title = computed(() => route.meta.title || '智深专属知识库')
const avatar = computed(() => (state.user?.nickname || '知').slice(0, 1))

onMounted(bootstrap)
</script>

<template>
  <div v-if="state.booting" class="boot-screen"><span>⌘</span><p>正在进入专属知识空间…</p></div>
  <LoginView v-else-if="!state.user" />
  <div v-else class="app-shell">
    <aside class="side-nav">
      <div class="brand"><span>⌘</span><div><strong>智深心流</strong><small>PERSON KNOW</small></div></div>
      <nav><RouterLink v-for="item in nav" :key="item.to" :to="item.to"><span>{{ item.icon }}</span><b>{{ item.label }}</b></RouterLink></nav>
      <RouterLink class="side-profile" to="/profile"><span>{{ avatar }}</span><div><strong>{{ state.user.nickname || '微信用户' }}</strong><small><i :class="{ online: state.online }" /> 专属实例在线</small></div></RouterLink>
    </aside>
    <main class="workspace">
      <header class="topbar"><div><small>个人知识架构工作台</small><h1>{{ title }}</h1></div><div class="topbar-status"><span :class="{ online: state.online }" />{{ state.online ? '系统在线' : '连接中断' }}</div></header>
      <RouterView v-slot="{ Component }"><Transition name="view" mode="out-in"><component :is="Component" /></Transition></RouterView>
    </main>
  </div>
  <ToastHost />
</template>
