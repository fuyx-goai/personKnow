<script setup>
import { computed, reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import { api } from '../api'
import { allLibraries, loadWorkspace, notify, state } from '../store'

const router = useRouter()
const query = ref('')
const category = ref('')
const modalOpen = ref(false)
const saving = ref(false)
const form = reactive({ name: '', category: '专业知识', description: '', visibility: 'private' })
const categories = computed(() => [...new Set(allLibraries.value.map((item) => item.category).filter(Boolean))])
const visible = computed(() => allLibraries.value.filter((item) => {
  if (category.value && item.category !== category.value) return false
  const keyword = query.value.trim().toLowerCase()
  return !keyword || `${item.name} ${item.description}`.toLowerCase().includes(keyword)
}))
const totals = computed(() => allLibraries.value.reduce((sum, item) => ({
  documents: sum.documents + Number(item.document_count || 0),
  chunks: sum.chunks + Number(item.chunk_count || 0),
}), { documents: 0, chunks: 0 }))

function formatBytes(value = 0) {
  if (value < 1024) return `${value} B`
  if (value < 1048576) return `${(value / 1024).toFixed(1)} KB`
  return `${(value / 1048576).toFixed(1)} MB`
}

async function createLibrary() {
  if (!form.name.trim()) return
  saving.value = true
  try {
    await api.createLibrary({ ...form, name: form.name.trim() })
    await loadWorkspace()
    Object.assign(form, { name: '', category: '专业知识', description: '', visibility: 'private' })
    modalOpen.value = false
    notify('知识库已创建')
  } catch (error) { notify(error.message, 'bad') }
  finally { saving.value = false }
}

async function remove(item) {
  if (!confirm(`确认删除「${item.name}」及其中全部资料？`)) return
  try { await api.deleteLibrary(item.id); await loadWorkspace(); notify('已提交异步删除') }
  catch (error) { notify(error.message, 'bad') }
}
</script>

<template>
  <section class="screen libraries-screen">
    <header class="screen-hero">
      <div><span class="eyebrow">KNOWLEDGE SYSTEM</span><h2>专属知识库 <b>{{ allLibraries.length }} 个体系</b></h2><p>高精度语义检索 · 私有与公开知识独立管理</p></div>
      <button class="primary" type="button" @click="modalOpen = true">＋ 新建知识库</button>
    </header>
    <div class="metric-grid">
      <article><span>▧</span><div><small>文档总量</small><strong>{{ totals.documents }}</strong><small>份资料</small></div></article>
      <article><span>◎</span><div><small>总切块</small><strong>{{ totals.chunks }}</strong><small>Chunks</small></div></article>
      <article><span>⌁</span><div><small>Embedding</small><strong class="word">Hybrid</strong><small>混合检索</small></div></article>
    </div>
    <div class="filter-bar"><input v-model="query" placeholder="搜索知识库名称或描述…"><select v-model="category"><option value="">全部分类</option><option v-for="item in categories" :key="item">{{ item }}</option></select></div>
    <div v-if="!visible.length" class="empty-state">暂无匹配知识库</div>
    <div v-else class="library-grid">
      <article v-for="(item,index) in visible" :key="item.id" class="library-card" :style="{ '--accent-index': index % 4 }">
        <button v-if="!item.readOnly" class="delete-link" type="button" @click="remove(item)">删除</button>
        <header><span class="library-mark">⌘</span><div><h3>{{ item.name }} <em :class="{ public: item.readOnly }">{{ item.readOnly ? '公开只读' : '私有' }}</em></h3><small>{{ item.readOnly ? `来自 ${item.owner_nickname || '公开贡献者'}` : item.category }}</small></div></header>
        <p>{{ item.description || '已建立语义索引，可直接进入知识问答。' }}</p>
        <div class="library-meta"><span>▧ {{ item.document_count }} 个文件</span><span>◇ {{ formatBytes(item.storage_bytes) }}</span><span>⌁ {{ Number(item.indexed_tokens || 0).toLocaleString() }} Tokens</span></div>
        <footer><button type="button" @click="router.push({ path: '/documents', query: { library: item.id } })">{{ item.readOnly ? '查看资料' : '资料管理' }}</button><button class="primary" type="button" @click="router.push({ path: '/ask', query: { library: item.id } })">进入问答 ›</button></footer>
      </article>
    </div>
    <div v-if="modalOpen" class="modal-mask" @click.self="modalOpen = false"><form class="modal-card" @submit.prevent="createLibrary"><header><div><h3>新建知识库</h3><p>建立独立的检索边界</p></div><button type="button" @click="modalOpen = false">×</button></header><label>知识库名称<input v-model="form.name" maxlength="120" placeholder="例如：AI 架构与大模型系统"></label><label>分类<input v-model="form.category" maxlength="40"></label><label>描述<textarea v-model="form.description" maxlength="300" /></label><div class="segmented"><button type="button" :class="{ active: form.visibility === 'private' }" @click="form.visibility = 'private'">私有</button><button type="button" :class="{ active: form.visibility === 'public' }" @click="form.visibility = 'public'">公开只读</button></div><button class="primary wide" :disabled="saving || !form.name.trim()">{{ saving ? '正在创建…' : '创建知识库' }}</button></form></div>
  </section>
</template>
