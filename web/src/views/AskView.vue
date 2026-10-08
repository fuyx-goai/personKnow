<script setup>
import { computed, nextTick, onBeforeUnmount, onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import { api, streamChat } from '../api'
import { allLibraries, notify, state } from '../store'
import { renderMarkdown } from '../utils/markdown'

const route = useRoute()
const question = ref('')
const busy = ref(false)
const historyOpen = ref(false)
const sessions = ref([])
const messages = ref([])
const currentSessionId = ref('')
const libraryId = ref(typeof route.query.library === 'string' ? route.query.library : '')
const composer = ref(null)
let controller = null
const libraries = computed(() => [{ id: '', name: '全域专属知识库（混合检索）' }, ...allLibraries.value])
const suggestions = ['总结当前知识库的核心主题', '有哪些内容值得继续整理？', '解释资料中的关键技术术语']

async function loadSessions() {
  try { sessions.value = (await api.chatSessions()).sessions || [] }
  catch (error) { notify(error.message, 'bad') }
}

function newChat() {
  controller?.abort()
  currentSessionId.value = ''
  messages.value = []
  busy.value = false
  historyOpen.value = false
}

async function ensureSession(title) {
  if (currentSessionId.value) return currentSessionId.value
  const command = libraryId.value
    ? { scope_type: 'single_library', library_id: libraryId.value, title: title.slice(0, 36) }
    : { scope_type: 'global', title: title.slice(0, 36) }
  const session = await api.createChatSession(command)
  currentSessionId.value = session.id
  sessions.value.unshift(session)
  return session.id
}

async function openSession(session) {
  try {
    const result = await api.chatHistory(session.id)
    currentSessionId.value = session.id
    libraryId.value = result.session.library_id || ''
    messages.value = (result.messages || []).map((message) => ({ ...message, references: message.references || [] }))
    historyOpen.value = false
    keepInView(true)
  } catch (error) { notify(error.message, 'bad') }
}

async function send(preset) {
  const value = (preset ?? question.value).trim()
  if (!value || busy.value) return
  try {
    const sessionId = await ensureSession(value)
    messages.value.push({ id: `u-${Date.now()}`, role: 'user', content: value, references: [] })
    const assistant = { id: `a-${Date.now()}`, role: 'assistant', content: '', references: [], streaming: true, usage: null }
    messages.value.push(assistant)
    question.value = ''
    busy.value = true
    controller = new AbortController()
    await streamChat(sessionId, value, {
      onDelta: (delta) => { assistant.content += delta; keepInView() },
      onReference: (reference) => assistant.references.push(reference),
      onUsage: (usage) => { assistant.usage = usage },
      onError: (error) => { assistant.error = error.message || '问答生成失败' },
    }, controller.signal)
    assistant.streaming = false
  } catch (error) {
    if (error.name !== 'AbortError') notify(error.message, 'bad')
  } finally {
    busy.value = false
    controller = null
    keepInView(true)
  }
}

function stop() { controller?.abort(); const last = messages.value.at(-1); if (last) last.streaming = false }
function resizeComposer() { if (composer.value) { composer.value.style.height = 'auto'; composer.value.style.height = `${Math.min(composer.value.scrollHeight, 150)}px` } }
function keepInView(force = false) { nextTick(() => { const root = document.scrollingElement; if (root && (force || root.scrollHeight - root.scrollTop - root.clientHeight < 280)) root.scrollTop = root.scrollHeight }) }
function score(value) { return `${Math.round(Number(value || 0) * 100)}%` }

onMounted(loadSessions)
onBeforeUnmount(() => controller?.abort())
</script>

<template>
  <section class="ask-screen">
    <div class="chat-toolbar"><button type="button" @click="historyOpen = !historyOpen">↻ <b>{{ sessions.length }}</b></button><select v-model="libraryId" @change="newChat"><option v-for="item in libraries" :key="item.id" :value="item.id">◈ {{ item.name }}</option></select><button class="new-chat" type="button" @click="newChat">＋ 新建问答</button></div>
    <div v-if="!messages.length" class="ask-welcome"><span>⌘</span><h2>有什么我能帮你的吗？</h2><p>我会先检索指定知识库，再给出带出处的回答。</p><div><button v-for="item in suggestions" :key="item" type="button" @click="send(item)">✦ {{ item }}</button></div></div>
    <div v-else class="message-list"><article v-for="message in messages" :key="message.id" :class="['message-row', message.role]"><span class="message-avatar">{{ message.role === 'user' ? '你' : '⌘' }}</span><div class="message-card"><div v-if="message.role === 'assistant'" class="markdown" v-html="renderMarkdown(message.content || (message.streaming ? '正在检索知识库…' : ''))" /><p v-else>{{ message.content }}</p><span v-if="message.streaming" class="typing">● ● ●</span><p v-if="message.error" class="message-error">{{ message.error }}</p><div v-if="message.references?.length" class="reference-list"><h4>原资料溯源依据（{{ message.references.length }} 处匹配）</h4><article v-for="reference in message.references" :key="reference.chunk_id"><header><strong>{{ reference.source_name }}</strong><em>{{ score(reference.similarity) }} 相似</em></header><p>{{ reference.excerpt }}</p><small>{{ reference.location_label }}</small></article></div><footer v-if="message.role === 'assistant' && message.content"><span>{{ state.config.chatModel || 'DeepSeek-V3' }}</span><span v-if="message.usage">{{ Number(message.usage.input_tokens + message.usage.output_tokens).toLocaleString() }} Tokens</span></footer></div></article></div>
    <form class="web-composer" @submit.prevent="send()"><textarea ref="composer" v-model="question" rows="1" placeholder="询问知识库中的技术架构、读书卡片或操作规范…" :disabled="busy" @input="resizeComposer" @keydown.enter.exact.prevent="send()" /><div><span>♢ 目标库：{{ libraries.find(item => item.id === libraryId)?.name }}</span><button v-if="busy" class="stop" type="button" @click="stop">■</button><button v-else class="send" type="submit" :disabled="!question.trim()">➤</button></div></form>
    <div v-if="historyOpen" class="history-drawer"><header><h3>历史问答</h3><button @click="historyOpen = false">×</button></header><button v-for="session in sessions" :key="session.id" class="history-item" @click="openSession(session)"><strong>{{ session.title || '未命名问答' }}</strong><small>{{ new Date(session.updated_at).toLocaleString() }}</small></button><p v-if="!sessions.length">还没有历史问答</p></div>
  </section>
</template>
