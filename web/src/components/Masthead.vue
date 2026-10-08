<script setup>
import { computed } from 'vue'
import { useRoute } from 'vue-router'
import { state } from '../store'
import AppIcon from './AppIcon.vue'

const route = useRoute()
const title = computed(() => route.meta.title || '个人知识库')
const note = computed(() => route.meta.note || '')
const model = computed(() => state.chatModel || 'AI 助手')
</script>

<template>
  <header class="topbar">
    <div class="topbar__page">
      <button class="icon-button" type="button" title="侧栏"><AppIcon name="panel" :size="19" /></button>
      <span class="topbar__divider" />
      <div class="topbar__copy">
        <strong>{{ title }}</strong>
        <small>{{ note }}</small>
      </div>
    </div>

    <div class="topbar__tools">
      <span class="status" :class="{ 'is-online': state.online }">
        <i />{{ state.online ? '服务在线' : '离线模式' }}
      </span>
      <span class="model"><AppIcon name="sparkle" :size="15" />{{ model }}</span>
      <button class="icon-button" type="button" title="更多"><AppIcon name="more" :size="19" /></button>
    </div>
  </header>
</template>

<style scoped>
.topbar {
  position: sticky;
  top: 0;
  z-index: 8;
  display: flex;
  align-items: center;
  justify-content: space-between;
  height: var(--topbar-height);
  padding: 0 24px;
  border-bottom: 1px solid var(--line);
  background: rgba(255, 255, 255, 0.9);
  backdrop-filter: blur(18px);
}

.topbar__page, .topbar__tools, .model, .status { display: flex; align-items: center; }
.topbar__page { min-width: 0; gap: 12px; }
.topbar__tools { gap: 10px; }
.topbar__divider { width: 1px; height: 18px; background: var(--line); }
.topbar__copy { display: flex; min-width: 0; flex-direction: column; }
.topbar__copy strong { font-size: 14px; font-weight: 620; line-height: 1.35; }
.topbar__copy small { max-width: 34vw; overflow: hidden; color: var(--text-3); font-size: 11px; text-overflow: ellipsis; white-space: nowrap; }

.icon-button {
  display: grid;
  place-items: center;
  width: 34px;
  height: 34px;
  padding: 0;
  border: 0;
  border-radius: 10px;
  color: var(--text-2);
  background: transparent;
  cursor: pointer;
}

.icon-button:hover { color: var(--text); background: var(--surface-soft); }
.model, .status { gap: 6px; height: 32px; padding: 0 10px; border: 1px solid var(--line); border-radius: 999px; color: var(--text-2); background: #fff; font-size: 11px; }
.status i { width: 6px; height: 6px; border-radius: 50%; background: #b8bdc5; }
.status.is-online i { background: var(--success); box-shadow: 0 0 0 3px rgba(33, 163, 102, 0.1); }
.model { color: var(--primary); background: var(--surface-blue); border-color: #e0e9ff; }

@media (max-width: 720px) {
  .topbar { padding-inline: 14px; }
  .topbar__copy small, .model { display: none; }
}

@media (max-width: 480px) { .status { display: none; } }
</style>
