<script setup>
// ChunkCard —— 一张"索引卡"，展示一个知识片段
//
// 为什么不渲染 Markdown？藏书页一屏可能有上百张卡，
// 保持纯文本既能原样呈现笔记里的缩进和代码块，渲染开销也最低。
import { computed, ref } from 'vue'
import { bytes } from '../utils/format'

const props = defineProps({
  chunk: { type: Object, required: true },
  index: { type: Number, default: 0 },
})

const expanded = ref(false)

// 长片段先折起来，避免一条长笔记把整页刷满
const LIMIT = 220

const content = computed(() => props.chunk.content || '')
const isLong = computed(() => content.value.length > LIMIT)

const shown = computed(() =>
  expanded.value || !isLong.value ? content.value : `${content.value.slice(0, LIMIT)}…`,
)

// 入场动画错开一拍，卡片像被一张张摆上来
const delay = computed(() => `${Math.min(props.index, 12) * 32}ms`)
</script>

<template>
  <article class="chunk" :style="{ animationDelay: delay }">
    <header class="chunk__head">
      <span class="badge">{{ chunk.source }}</span>
      <span class="chunk__meta is-mono">{{ bytes(chunk.bytes) }}</span>
      <span class="chunk__meta is-mono">#{{ (chunk.id || '').slice(0, 6) }}</span>
    </header>

    <pre class="chunk__body">{{ shown }}</pre>

    <button v-if="isLong" class="chunk__more" type="button" @click="expanded = !expanded">
      {{ expanded ? '收起' : `展开全文 · ${content.length} 字` }}
    </button>
  </article>
</template>

<style scoped>
.chunk {
  position: relative;
  padding: 1rem 1.1rem 1rem 1.3rem;
  border: 1px solid var(--line);
  border-radius: 16px;
  background: var(--surface);
  box-shadow: var(--shadow-sm);
  animation: rise 0.5s var(--ease) backwards;
  transition: transform 0.2s var(--ease), box-shadow 0.2s var(--ease);
}

.chunk:hover {
  transform: translateY(-2px);
  border-color: #d7e0f0;
  box-shadow: 0 12px 30px rgba(20, 28, 40, 0.07);
}

.chunk::before {
  content: '';
  position: absolute;
  top: 0;
  left: 20px;
  width: 36px;
  height: 3px;
  border-radius: 0 0 99px 99px;
  background: var(--primary);
}

.chunk__head {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 0.5rem;
  margin-bottom: 0.7rem;
}

.chunk__meta {
  font-size: 0.7rem;
  color: var(--text-3);
}

.chunk__body {
  margin: 0;
  font-family: var(--sans);
  font-size: 0.9rem;
  line-height: 1.85;
  color: var(--text-2);
  white-space: pre-wrap;
  overflow-wrap: anywhere;
}

.chunk__more {
  margin-top: 0.7rem;
  padding: 0;
  border: 0;
  background: none;
  color: var(--primary);
  font-family: var(--sans);
  font-size: 0.72rem;
  letter-spacing: 0.06em;
  cursor: pointer;
  border-bottom: 1px dashed currentColor;
}

.chunk__more:hover {
  color: var(--primary-hover);
}
</style>
