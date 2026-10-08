<script setup>
import { computed, onMounted, reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import { api } from '../api'
import { loadWorkspace, logout, notify, state } from '../store'

const router = useRouter()
const tab = ref('usage')
const sessions = ref([])
const audits = ref([])
const libraryId = ref('')
const settings = reactive({ chunk_size: 800, chunk_overlap: 100, top_k: 5, similarity_threshold: .3 })
const reranker = ref(true)
const saving = ref(false)
const usage = computed(() => state.usage || {})
const storagePercent = computed(() => percent(usage.value.storage_bytes, usage.value.storage_quota_bytes))
const tokenPercent = computed(() => percent(usage.value.total_tokens, usage.value.monthly_token_quota))

function percent(value, total) { return total ? Math.min(100, Math.round((Number(value) / Number(total)) * 100)) : 0 }
function bytes(value = 0) { return value < 1048576 ? `${(value / 1024).toFixed(1)} KB` : `${(value / 1048576).toFixed(1)} MB` }

async function loadAccountData() {
  try {
    const [sessionResult, auditResult] = await Promise.all([api.sessions(), api.auditLogs()])
    sessions.value = sessionResult.sessions || []
    audits.value = auditResult.audit_logs || []
    if (!libraryId.value && state.ownedLibraries.length) libraryId.value = state.ownedLibraries[0].id
    if (libraryId.value) await loadSettings()
  } catch (error) { notify(error.message, 'bad') }
}

async function loadSettings() {
  if (!libraryId.value) return
  try { Object.assign(settings, await api.librarySettings(libraryId.value)) }
  catch (error) { notify(error.message, 'bad') }
}

async function saveSettings() {
  saving.value = true
  try { await api.updateLibrarySettings(libraryId.value, settings); notify('检索参数已保存') }
  catch (error) { notify(error.message, 'bad') }
  finally { saving.value = false }
}

async function exportBackup() {
  try {
    const documents = (await api.documents({ limit: 100 })).documents || []
    const backup = { exported_at: new Date().toISOString(), user: state.user, libraries: state.ownedLibraries, documents, usage: state.usage, audit_logs: audits.value }
    const url = URL.createObjectURL(new Blob([JSON.stringify(backup, null, 2)], { type: 'application/json' }))
    const link = document.createElement('a')
    link.href = url
    link.download = `personknow-backup-${new Date().toISOString().slice(0, 10)}.json`
    link.click()
    URL.revokeObjectURL(url)
    notify('知识库备份已下载')
  } catch (error) { notify(error.message, 'bad') }
}

async function signOut() {
  if (!confirm('确认退出当前 Web 登录？')) return
  await logout()
  router.replace('/ask')
}

onMounted(async () => { await loadWorkspace(); await loadAccountData() })
</script>

<template>
  <section class="screen profile-screen">
    <div class="profile-sheet">
      <header class="profile-title"><h2>个人空间设置与知识库状态</h2><span>Personal Pro</span></header>
      <div class="identity-card"><div class="profile-avatar">{{ (state.user?.nickname || '知').slice(0,1) }}</div><div><h3>{{ state.user?.nickname || '微信知识库用户' }}</h3><p>{{ state.user?.id }}</p><small>个人知识架构师 · <b>专属实例在线</b></small></div></div>
      <div class="profile-tabs"><button :class="{ active: tab === 'usage' }" @click="tab = 'usage'">容量与用量</button><button :class="{ active: tab === 'rag' }" @click="tab = 'rag'">RAG检索微调</button><button :class="{ active: tab === 'security' }" @click="tab = 'security'">账号与安全</button></div>
      <div v-if="tab === 'usage'" class="profile-pane"><article class="usage-panel"><header><span>知识库存储空间</span><strong>{{ bytes(usage.storage_bytes) }} / {{ bytes(usage.storage_quota_bytes) }}</strong></header><div><i :style="{ width: `${storagePercent}%` }" /></div><p>包含原文件、文本版本与索引元数据</p></article><article class="usage-panel token"><header><span>本月 Token 用量</span><strong>{{ Number(usage.total_tokens || 0).toLocaleString() }} / {{ Number(usage.monthly_token_quota || 0).toLocaleString() }}</strong></header><div><i :style="{ width: `${tokenPercent}%` }" /></div><p>Embedding {{ usage.embedding_tokens || 0 }} · 输入 {{ usage.input_tokens || 0 }} · 输出 {{ usage.output_tokens || 0 }}</p></article><div class="profile-summary"><article><strong>{{ state.ownedLibraries.length }}</strong><span>私有知识库</span></article><article><strong>{{ sessions.length }}</strong><span>活跃设备会话</span></article></div><section class="audit-panel"><header><h3>最近操作记录</h3><span>审计日志</span></header><article v-for="item in audits" :key="item.id"><i /><div><strong>{{ item.action }}</strong><small>{{ item.resource_type }} · {{ item.result }}</small></div><time>{{ new Date(item.occurred_at).toLocaleString() }}</time></article><p v-if="!audits.length">暂无操作记录</p></section></div>
      <div v-else-if="tab === 'rag'" class="profile-pane rag-pane"><template v-if="state.ownedLibraries.length"><label>当前知识库<select v-model="libraryId" @change="loadSettings"><option v-for="item in state.ownedLibraries" :key="item.id" :value="item.id">{{ item.name }}</option></select></label><label>主力对话与合成模型<input :value="state.config.chatModel || 'DeepSeek-V3（混合长检索优化）'" readonly></label><label class="range-label"><span>Top-K 候选召回片段数 <b>{{ settings.top_k }} 块</b></span><input v-model.number="settings.top_k" type="range" min="1" max="20"></label><label class="range-label"><span>相似度过滤阈值 <b>{{ Number(settings.similarity_threshold).toFixed(2) }}</b></span><input v-model.number="settings.similarity_threshold" type="range" min="0" max="1" step=".01"></label><div class="switch-panel"><div><strong>BGE-Reranker 二次重排</strong><small>提高专有术语和代码段检索精准度</small></div><input v-model="reranker" type="checkbox" role="switch"></div><button class="primary wide" :disabled="saving" @click="saveSettings">{{ saving ? '正在保存…' : '保存检索设置' }}</button></template><p v-else class="empty-state">创建私有知识库后可调整检索参数。</p></div>
      <div v-else class="profile-pane security-pane"><div class="trusted-card"><span>✓</span><div><h3>Web 受信任扫码会话</h3><p>已通过微信小程序确认，敏感知识仅在受授权请求中解密展现。</p></div></div><article v-for="item in sessions" :key="item.id" class="session-card"><div><strong>{{ item.device_label || '浏览器设备' }}</strong><small>{{ item.client_type }} · 有效至 {{ new Date(item.expires_at).toLocaleString() }}</small></div><span>在线</span></article><button class="backup-btn" @click="exportBackup">⇩ 导出全量知识库备份 (JSON+Markdown)</button></div>
      <button class="logout-btn" @click="signOut">退出当前微信登录</button>
    </div>
  </section>
</template>
