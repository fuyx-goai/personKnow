<script setup>
// LibraryView —— 藏书：逐段翻阅，按来源清理
//
// 这是"管理"里最实的一块：看得见每一段原文、找得到它出自哪份笔记、
// 能只删这一份，也能一次清空。删除都会先过一次二次确认。
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import { api } from '../api'
import { ensureChunks, invalidateChunks, notify, refreshStats, shelves, state } from '../store'
import { bytes } from '../utils/format'
import ChunkCard from '../components/ChunkCard.vue'
import ConfirmDialog from '../components/ConfirmDialog.vue'
import AppIcon from '../components/AppIcon.vue'
import '../styles/library.css'

const route = useRoute()

const keyword = ref('')
const sourceFilter = ref('')
const busy = ref(false)
const dialog = ref({ open: false, title: '', body: '', confirmText: '', action: null })

// 从总览点某个来源跳进来时带着 ?source=xxx，直接筛好省一步
watch(
  () => route.query.source,
  (value) => {
    sourceFilter.value = typeof value === 'string' ? value : ''
  },
  { immediate: true },
)

const filtered = computed(() => {
  const kw = keyword.value.trim().toLowerCase()
  return state.chunks.filter((chunk) => {
    if (sourceFilter.value && chunk.source !== sourceFilter.value) return false
    if (!kw) return true
    return (
      chunk.content.toLowerCase().includes(kw) || chunk.source.toLowerCase().includes(kw)
    )
  })
})

// 按来源分组，组内保持服务端返回的顺序
const groups = computed(() => {
  const order = shelves.value.map((s) => s.source)
  const map = new Map()

  for (const chunk of filtered.value) {
    if (!map.has(chunk.source)) map.set(chunk.source, [])
    map.get(chunk.source).push(chunk)
  }

  return [...map.entries()]
    .map(([source, items]) => ({
      source,
      items,
      bytes: items.reduce((sum, c) => sum + (c.bytes || 0), 0),
    }))
    .sort((a, b) => order.indexOf(a.source) - order.indexOf(b.source))
})

// 被筛掉的那份来源要是已经不存在了（比如刚删完），把筛选项复位，
// 否则页面会停在一个"永远为空"的筛选上，让人以为库空了
watch(shelves, (list) => {
  if (sourceFilter.value && !list.some((s) => s.source === sourceFilter.value)) {
    sourceFilter.value = ''
  }
})

onMounted(() => {
  ensureChunks().catch((e) => notify(`读取片段失败：${e.message}`, 'bad'))
})

async function reload() {
  busy.value = true
  try {
    await Promise.all([refreshStats(), ensureChunks(true)])
    notify('已重新载入')
  } catch (e) {
    notify(`载入失败：${e.message}`, 'bad')
  } finally {
    busy.value = false
  }
}

function askRemoveSource(source) {
  const count = state.chunks.filter((c) => c.source === source).length
  dialog.value = {
    open: true,
    title: '删掉这一份？',
    body: `将删除「${source}」的 ${count} 段片段，不可撤销。磁盘上的原文件不动，需要时可以重新摄入。`,
    confirmText: '删除',
    action: () => remove(source),
  }
}

function askWipe() {
  dialog.value = {
    open: true,
    title: '清空整个知识库？',
    body: `当前库里有 ${state.total} 段片段，清空后必须重新摄入才能恢复。`,
    confirmText: '清空整库',
    action: () => remove(''),
  }
}

// remove 传空来源即清空整库（后端约定）
async function remove(source) {
  try {
    const result = await api.removeChunks(source)
    notify(
      source
        ? `已删除「${source}」的 ${result.deleted} 段`
        : `已清空整库，共删除 ${result.deleted} 段`,
    )
    invalidateChunks()
    await Promise.all([refreshStats(), ensureChunks(true)])
  } catch (e) {
    notify(`删除失败：${e.message}`, 'bad')
  }
}

async function runConfirm() {
  const action = dialog.value.action
  if (!action) return
  try {
    await action()
  } finally {
    dialog.value.action = null
  }
}
</script>

<template>
  <section class="view">
    <header class="library-head">
      <div>
        <span class="library-head__icon"><AppIcon name="library" :size="22" /></span>
        <div><h1>资料库</h1><p>搜索、浏览并管理已经建立索引的知识内容。</p></div>
      </div>
      <RouterLink class="btn btn--primary" to="/ingest"><AppIcon name="plus" :size="17" />添加资料</RouterLink>
    </header>

    <div class="toolbar">
      <input
        v-model="keyword"
        class="input toolbar__search"
        type="search"
        placeholder="搜索正文或来源…"
        autocomplete="off"
      />

      <select v-model="sourceFilter" class="input toolbar__select">
        <option value="">全部来源</option>
        <option v-for="item in shelves" :key="item.source" :value="item.source">
          {{ item.source }}（{{ item.count }}）
        </option>
      </select>

      <span class="toolbar__count is-mono">
        {{ filtered.length }} / {{ state.total }} 段
      </span>

      <div class="toolbar__actions">
        <button class="btn btn--ghost btn--tiny" type="button" :disabled="busy" @click="reload">
          <AppIcon name="refresh" :size="15" />{{ busy ? '载入中…' : '重新载入' }}
        </button>
        <button
          class="btn btn--danger-ghost btn--tiny"
          type="button"
          :disabled="!state.total"
          @click="askWipe"
        >
          <AppIcon name="trash" :size="15" />清空整库
        </button>
      </div>
    </div>

    <div v-if="state.chunksLoading && !state.chunksLoaded" class="empty">正在成册…</div>

    <div v-else-if="!state.chunks.length" class="empty">
      <span class="empty__mark">空</span>
      <p class="empty__text">这里还什么都没有。先去「摄入」页把笔记收进来吧。</p>
      <RouterLink class="btn btn--primary" to="/ingest">去摄入</RouterLink>
    </div>

    <div v-else-if="!filtered.length" class="empty">
      <span class="empty__mark">无</span>
      <p class="empty__text">没有匹配的片段，换个词试试，或者把筛选项清掉。</p>
    </div>

    <div v-else class="shelves">
      <section v-for="group in groups" :key="group.source" class="shelf">
        <header class="shelf__head">
          <h2 class="shelf__name">{{ group.source }}</h2>
          <span class="shelf__meta is-mono">
            {{ group.items.length }} 段 · {{ bytes(group.bytes) }}
          </span>
          <button
            class="btn btn--danger-ghost btn--tiny"
            type="button"
            @click="askRemoveSource(group.source)"
          >
            <AppIcon name="trash" :size="14" />删除此来源
          </button>
        </header>

        <div class="shelf__cards">
          <ChunkCard
            v-for="(chunk, i) in group.items"
            :key="chunk.id"
            :chunk="chunk"
            :index="i"
          />
        </div>
      </section>
    </div>

    <ConfirmDialog
      v-model:open="dialog.open"
      :title="dialog.title"
      :body="dialog.body"
      :confirm-text="dialog.confirmText"
      @confirm="runConfirm"
    />
  </section>
</template>
