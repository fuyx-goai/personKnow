<script setup>
import { computed } from 'vue'
import { shelves, state } from '../store'
import AppIcon from './AppIcon.vue'

const items = [
  { to: '/overview', label: '概览', icon: 'home' },
  { to: '/ingest', label: '知识摄入', icon: 'upload' },
  { to: '/ask', label: '知识问答', icon: 'chat' },
  { to: '/library', label: '资料库', icon: 'library' },
]

const recentSources = computed(() => shelves.value.slice(0, 6))
</script>

<template>
  <aside class="sidebar">
    <RouterLink class="brand" to="/ask" aria-label="Knowledge 首页">
      <span class="brand__mark"><AppIcon name="sparkle" :size="20" :stroke-width="2" /></span>
      <span class="brand__name">Knowledge</span>
    </RouterLink>

    <nav class="nav" aria-label="主导航">
      <RouterLink v-for="item in items" :key="item.to" class="nav__item" :to="item.to">
        <AppIcon :name="item.icon" :size="20" />
        <span>{{ item.label }}</span>
      </RouterLink>
    </nav>

    <section class="recent">
      <p class="recent__title">最近资料</p>
      <RouterLink
        v-for="source in recentSources"
        :key="source.source"
        class="recent__item"
        :to="{ path: '/library', query: { source: source.source } }"
      >
        <span class="recent__bubble"><AppIcon name="file" :size="14" /></span>
        <span class="recent__name">{{ source.source }}</span>
        <span class="recent__count">{{ source.count }}</span>
      </RouterLink>
      <p v-if="!recentSources.length" class="recent__empty">摄入资料后会显示在这里</p>
    </section>

    <div class="account">
      <span class="account__avatar">K</span>
      <span class="account__copy">
        <strong>我的知识库</strong>
        <small><i :class="{ 'is-on': state.online }" />{{ state.online ? '服务已连接' : '服务未连接' }}</small>
      </span>
      <AppIcon name="more" :size="18" />
    </div>
  </aside>
</template>

<style scoped>
.sidebar {
  position: sticky;
  top: 0;
  z-index: 10;
  display: flex;
  flex-direction: column;
  width: var(--sidebar-width);
  height: 100vh;
  padding: 18px 14px 14px;
  border-right: 1px solid var(--line);
  background: var(--sidebar);
  overflow: hidden;
}

.brand {
  display: flex;
  align-items: center;
  gap: 11px;
  height: 42px;
  padding: 0 8px;
  color: var(--text);
  text-decoration: none;
}

.brand__mark {
  display: grid;
  place-items: center;
  width: 32px;
  height: 32px;
  border-radius: 10px;
  color: #fff;
  background: var(--primary);
  box-shadow: 0 6px 14px rgba(23, 105, 255, 0.22);
}

.brand__name { font-size: 18px; font-weight: 720; letter-spacing: -0.02em; }

.nav { display: grid; gap: 5px; margin-top: 24px; }

.nav__item {
  display: flex;
  align-items: center;
  gap: 12px;
  height: 44px;
  padding: 0 12px;
  border-radius: 11px;
  color: #2f3339;
  font-size: 14px;
  text-decoration: none;
  transition: background 0.18s, color 0.18s;
}

.nav__item:hover { background: #eceef1; }
.nav__item.router-link-active { color: var(--primary); background: #e9f0ff; font-weight: 600; }

.recent { min-height: 0; margin-top: 26px; overflow: hidden; }
.recent__title { margin: 0 10px 10px; color: var(--text-3); font-size: 12px; }

.recent__item {
  display: grid;
  grid-template-columns: 24px minmax(0, 1fr) auto;
  align-items: center;
  gap: 8px;
  height: 38px;
  padding: 0 10px;
  border-radius: 9px;
  color: var(--text-2);
  font-size: 13px;
  text-decoration: none;
}

.recent__item:hover { color: var(--text); background: #eceef1; }
.recent__bubble { display: grid; place-items: center; width: 24px; height: 24px; border-radius: 8px; color: #6684bd; background: #e7edfa; }
.recent__name { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.recent__count { color: #b0b4bb; font-size: 11px; }
.recent__empty { margin: 12px 10px; color: #b0b4bb; font-size: 12px; line-height: 1.6; }

.account {
  display: grid;
  grid-template-columns: 34px minmax(0, 1fr) 18px;
  align-items: center;
  gap: 10px;
  margin-top: auto;
  padding: 10px;
  border-top: 1px solid var(--line);
  color: var(--text-3);
}

.account__avatar { display: grid; place-items: center; width: 34px; height: 34px; border-radius: 50%; color: #fff; background: linear-gradient(145deg, #59677d, #252b35); font-weight: 650; }
.account__copy { display: flex; min-width: 0; flex-direction: column; }
.account__copy strong { overflow: hidden; color: var(--text); font-size: 13px; font-weight: 600; text-overflow: ellipsis; white-space: nowrap; }
.account__copy small { display: flex; align-items: center; gap: 5px; color: var(--text-3); font-size: 11px; }
.account__copy i { width: 6px; height: 6px; border-radius: 50%; background: #bec2c8; }
.account__copy i.is-on { background: var(--success); }

@media (max-width: 800px) {
  .sidebar { align-items: center; padding-inline: 10px; }
  .brand { padding: 0; }
  .brand__name, .nav__item span, .recent, .account__copy, .account > .app-icon { display: none; }
  .nav { width: 100%; }
  .nav__item { justify-content: center; padding: 0; }
  .account { display: flex; justify-content: center; width: 100%; padding-inline: 0; }
}
</style>
