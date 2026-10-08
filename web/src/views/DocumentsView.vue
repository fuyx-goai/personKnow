<script setup>
import { computed, onMounted, reactive, ref } from 'vue'
import { useRoute } from 'vue-router'
import { api, pollIndexJob } from '../api'
import { allLibraries, loadWorkspace, notify } from '../store'

const route = useRoute()
const documents = ref([])
const loading = ref(true)
const query = ref('')
const libraryId = ref(typeof route.query.library === 'string' ? route.query.library : '')
const working = ref('')
const fileInput = ref(null)
const editor = reactive({ open: false, saving: false, document: null, displayName: '', tags: '', text: '' })
const libraryMap = computed(() => new Map(allLibraries.value.map((item) => [item.id, item])))
const selectedOwner = computed(() => {
  const selected = libraryMap.value.get(libraryId.value)
  if (selected && !selected.readOnly) return selected
  return allLibraries.value.find((item) => !item.readOnly)
})
const visible = computed(() => documents.value.filter((item) => {
  if (libraryId.value && item.library_id !== libraryId.value) return false
  const keyword = query.value.trim().toLowerCase()
  return !keyword || `${item.display_name} ${item.summary} ${(item.tags || []).join(' ')}`.toLowerCase().includes(keyword)
}))
const statuses = { queued: '等待索引', processing: '正在索引', ready: '已切块就绪', failed: '索引失败', deleting: '正在删除', delete_failed: '删除失败' }

async function loadDocuments() {
  loading.value = true
  try { documents.value = (await api.documents({ limit: 100 })).documents || [] }
  catch (error) { notify(error.message, 'bad') }
  finally { loading.value = false }
}

function formatBytes(value = 0) {
  if (value < 1024) return `${value} B`
  if (value < 1048576) return `${(value / 1024).toFixed(1)} KB`
  return `${(value / 1048576).toFixed(1)} MB`
}

async function uploadFile(event) {
  const file = event.target.files?.[0]
  if (!file || !selectedOwner.value) return notify('请先创建或选择私有知识库', 'bad')
  working.value = 'upload'
  try {
    const result = await api.uploadDocument(selectedOwner.value.id, file)
    notify('文件已上传，正在建立索引')
    waitForJob(result.job_id)
    await loadDocuments()
  } catch (error) { notify(error.message, 'bad') }
  finally { working.value = ''; event.target.value = '' }
}

async function waitForJob(jobId) {
  if (!jobId) return
  try {
    const job = await pollIndexJob(jobId)
    notify(job.status === 'ready' ? '索引已完成' : `索引失败：${job.error_message || '请重试'}`, job.status === 'ready' ? 'ok' : 'bad')
    await Promise.all([loadDocuments(), loadWorkspace()])
  } catch (error) { notify(error.message, 'bad') }
}

async function openEditor(document) {
  if (libraryMap.value.get(document.library_id)?.readOnly) return notify('公开资料仅支持查看', 'bad')
  working.value = document.id
  try {
    const result = await api.documentContent(document.id)
    Object.assign(editor, { open: true, document, displayName: document.display_name, tags: (document.tags || []).join('，'), text: result.text || '' })
  } catch (error) { notify(error.message, 'bad') }
  finally { working.value = '' }
}

async function saveEditor() {
  editor.saving = true
  try {
    const tags = editor.tags.split(/[，,]/).map((item) => item.trim()).filter(Boolean)
    await api.updateDocument(editor.document.id, { DisplayName: editor.displayName, Tags: tags })
    const result = await api.editDocument(editor.document.id, editor.text)
    editor.open = false
    notify('已保存，正在重建索引')
    waitForJob(result.job_id)
    await loadDocuments()
  } catch (error) { notify(error.message, 'bad') }
  finally { editor.saving = false }
}

async function reindex(document) {
  working.value = document.id
  try { const result = await api.reindexDocument(document.id); notify('重新索引已开始'); waitForJob(result.job_id) }
  catch (error) { notify(error.message, 'bad') }
  finally { working.value = '' }
}

async function remove(document) {
  if (!confirm(`彻底删除「${document.display_name}」及其向量？`)) return
  try { const result = await api.deleteDocument(document.id); waitForJob(result.job_id); await loadDocuments() }
  catch (error) { notify(error.message, 'bad') }
}

onMounted(loadDocuments)
</script>

<template>
  <section class="screen documents-screen">
    <header class="screen-hero"><div><span class="eyebrow">SOURCE OPERATIONS</span><h2>原资料文件操作台 <b>{{ documents.length }} 个文件</b></h2><p>支持原文件查看、在线内容修改、重命名、重新分块与删除</p></div><button class="primary" :disabled="working === 'upload'" @click="fileInput.click()">⇧ 上传/导入文件</button><input ref="fileInput" class="sr-only" type="file" accept=".pdf,.docx,.pptx,.md,.txt,.html,.csv" @change="uploadFile"></header>
    <div class="filter-bar"><input v-model="query" placeholder="搜索文档名、标签或内容摘要…"><select v-model="libraryId"><option value="">全部知识库</option><option v-for="item in allLibraries" :key="item.id" :value="item.id">{{ item.name }}{{ item.readOnly ? ' · 公开' : '' }}</option></select></div>
    <div v-if="loading" class="empty-state">正在读取原资料…</div><div v-else-if="!visible.length" class="empty-state">暂无匹配资料</div>
    <div v-else class="document-list"><article v-for="item in visible" :key="item.id" class="document-card"><header><span class="file-mark">{{ item.format.toUpperCase() }}</span><div><h3>{{ item.display_name }}</h3><p>{{ formatBytes(item.original_bytes) }} <em :class="item.status">{{ statuses[item.status] || item.status }}</em><em v-if="libraryMap.get(item.library_id)?.readOnly" class="public">公开只读</em></p></div></header><p class="document-summary">{{ item.summary || '索引完成后将在这里生成内容摘要。' }}</p><div class="tag-row"><span>所属知识库：{{ libraryMap.get(item.library_id)?.name || '未知' }}</span><b v-for="tag in item.tags" :key="tag">#{{ tag }}</b></div><div v-if="!libraryMap.get(item.library_id)?.readOnly" class="document-actions"><button :disabled="working === item.id" @click="reindex(item)">重新切块向量化</button><button :disabled="working === item.id" @click="openEditor(item)">在线编辑/重命名</button><button class="danger" @click="remove(item)">删除</button></div><footer><span>{{ item.chunk_count }} 切块 · {{ Number(item.indexed_tokens || 0).toLocaleString() }} Tokens</span><span>更新于 {{ new Date(item.updated_at).toLocaleString() }}</span></footer></article></div>
    <div v-if="editor.open" class="modal-mask" @click.self="editor.open = false"><form class="modal-card editor-card" @submit.prevent="saveEditor"><header><div><h3>在线编辑原资料</h3><p>保存后自动生成新内容版本并重新索引</p></div><button type="button" @click="editor.open = false">×</button></header><label>显示名称<input v-model="editor.displayName" maxlength="255"></label><label>标签<input v-model="editor.tags" placeholder="RAG，VectorDB，重排"></label><label>可编辑文本<textarea v-model="editor.text" class="editor-text" /></label><button class="primary wide" :disabled="editor.saving">{{ editor.saving ? '正在保存并重建索引…' : '保存修改' }}</button></form></div>
  </section>
</template>
