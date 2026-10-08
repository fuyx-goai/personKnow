<script setup>
import { computed, nextTick, onBeforeUnmount, ref } from 'vue'
import { api } from '../api'
import { notify } from '../store'
import { clock } from '../utils/format'
import { renderMarkdown } from '../utils/markdown'
import AppIcon from '../components/AppIcon.vue'
import ScoreBar from '../components/ScoreBar.vue'

const EXAMPLES = ['帮我总结知识库里的主要内容', 'goroutine 是什么？', '上下文压缩是怎么做的？', '有哪些内容值得继续整理？']
const question = ref('')
const busy = ref(false)
const thread = ref([])
const composer = ref(null)
let controller = null

const isEmpty = computed(() => thread.value.length === 0)

function resizeComposer() {
  if (!composer.value) return
  composer.value.style.height = 'auto'
  composer.value.style.height = `${Math.min(composer.value.scrollHeight, 140)}px`
}

async function ask(text) {
  const value = (text ?? question.value).trim()
  if (!value || busy.value) return
  thread.value.push({ id: `${Date.now()}-${thread.value.length}`, question: value, answer: '', references: [], error: '', streaming: true, at: clock() })
  const entry = thread.value.at(-1)
  question.value = ''
  busy.value = true
  controller = new AbortController()
  nextTick(resizeComposer)

  try {
    await api.askStream(value, {
      onDelta(delta) { entry.answer += delta; keepInView() },
      onReferences(refs) { entry.references = (refs || []).map((item) => ({ ...item, open: false })) },
    }, controller.signal)
  } catch (error) {
    entry.error = error.message
    notify(`问答失败：${error.message}`, 'bad')
  } finally {
    entry.streaming = false
    busy.value = false
    controller = null
    keepInView(true)
  }
}

function stop() { controller?.abort() }
function clearThread() { controller?.abort(); thread.value = []; busy.value = false }
function keepInView(force = false) {
  nextTick(() => {
    const doc = document.scrollingElement || document.documentElement
    const distance = doc.scrollHeight - doc.scrollTop - doc.clientHeight
    if (force || distance < 260) doc.scrollTop = doc.scrollHeight
  })
}

onBeforeUnmount(() => controller?.abort())
</script>

<template>
  <section class="view ask-view" :class="{ 'has-thread': !isEmpty }">
    <div v-if="isEmpty" class="welcome">
      <span class="welcome__logo"><AppIcon name="sparkle" :size="28" :stroke-width="1.7" /></span>
      <h1>有什么我能帮你的吗？</h1>
      <p>我会先检索你的知识库，再给出带出处的回答。</p>
      <div class="mode-switch" aria-label="问答模式">
        <span class="is-active">知识问答</span><span>全库检索</span>
      </div>
      <div class="suggestions">
        <small>你可以试试</small>
        <button v-for="example in EXAMPLES" :key="example" type="button" @click="ask(example)">
          <AppIcon name="sparkle" :size="15" />{{ example }}
        </button>
      </div>
    </div>

    <div v-else class="thread">
      <article v-for="entry in thread" :key="entry.id" class="turn">
        <div class="message message--user">
          <span class="avatar avatar--user">你</span>
          <div class="bubble"><p>{{ entry.question }}</p><small>{{ entry.at }}</small></div>
        </div>
        <div class="message message--assistant">
          <span class="avatar avatar--ai"><AppIcon name="sparkle" :size="16" /></span>
          <div class="answer">
            <p v-if="entry.error" class="answer__error">{{ entry.error }}</p>
            <div v-if="entry.answer" class="md" v-html="renderMarkdown(entry.answer)" />
            <p v-if="entry.streaming && !entry.answer" class="thinking">正在检索你的知识库…</p>
            <span v-if="entry.streaming" class="caret" />
            <div v-if="entry.references.length" class="refs">
              <p class="refs__title">参考资料 · {{ entry.references.length }}</p>
              <button v-for="(item, index) in entry.references" :key="index" class="ref" type="button" @click="item.open = !item.open">
                <span class="ref__head"><span class="badge">{{ item.source }}</span><ScoreBar :score="item.score" /></span>
                <span class="ref__text" :class="{ 'is-open': item.open }">{{ item.snippet }}</span>
                <small>{{ item.open ? '收起' : '展开原文' }}</small>
              </button>
            </div>
          </div>
        </div>
      </article>
    </div>

    <form class="composer" @submit.prevent="ask()">
      <textarea ref="composer" v-model="question" rows="1" placeholder="发消息，或按住空格说话…" :disabled="busy"
        @input="resizeComposer" @keydown.enter.exact.prevent="ask()" />
      <div class="composer__tools">
        <span class="composer__mode"><AppIcon name="database" :size="17" />知识库问答</span>
        <div>
          <button v-if="!isEmpty && !busy" class="tool-button" type="button" title="清空对话" @click="clearThread"><AppIcon name="trash" :size="18" /></button>
          <button v-if="busy" class="send-button is-stop" type="button" title="停止生成" @click="stop"><AppIcon name="stop" :size="16" /></button>
          <button v-else class="send-button" type="submit" title="发送" :disabled="!question.trim()"><AppIcon name="send" :size="18" /></button>
        </div>
      </div>
      <small class="composer__hint">内容由 AI 生成，请核对重要信息</small>
    </form>
  </section>
</template>

<style scoped>
.ask-view { min-height: calc(100vh - var(--topbar-height)); padding-bottom: 190px; }
.welcome { display: flex; width: min(720px, 100%); min-height: calc(100vh - 330px); margin: 0 auto; flex-direction: column; align-items: center; justify-content: center; text-align: center; animation: rise 0.5s var(--ease); }
.welcome__logo { display: grid; place-items: center; width: 56px; height: 56px; margin-bottom: 22px; border-radius: 18px; color: #fff; background: var(--primary); box-shadow: 0 14px 34px rgba(23, 105, 255, 0.22); }
.welcome h1 { margin: 0; font-size: clamp(28px, 4vw, 38px); font-weight: 700; letter-spacing: -0.035em; }
.welcome > p { margin: 10px 0 24px; color: var(--text-3); font-size: 14px; }
.mode-switch { display: grid; grid-template-columns: 1fr 1fr; width: 330px; padding: 4px; border-radius: 999px; background: #f2f3f5; color: var(--text-3); font-size: 14px; }
.mode-switch span { padding: 8px 12px; border-radius: 999px; }
.mode-switch .is-active { color: var(--text); background: #fff; box-shadow: var(--shadow-sm); font-weight: 600; }
.suggestions { display: flex; width: min(620px, 100%); margin-top: 48px; flex-wrap: wrap; justify-content: center; gap: 10px; }
.suggestions small { flex: 0 0 100%; color: var(--text-3); text-align: left; }
.suggestions button { display: flex; align-items: center; gap: 7px; padding: 10px 14px; border: 1px solid var(--line); border-radius: 12px; color: var(--text-2); background: #fff; cursor: pointer; transition: border-color .18s, background .18s, transform .18s; }
.suggestions button:hover { border-color: #cfdaef; color: var(--primary); background: var(--surface-blue); transform: translateY(-1px); }
.thread { width: min(820px, 100%); margin: 8px auto 0; }
.turn { display: grid; gap: 26px; margin-bottom: 42px; animation: rise .35s var(--ease); }
.message { display: flex; align-items: flex-start; gap: 12px; }
.message--user { justify-content: flex-end; }
.message--user .avatar { order: 2; }
.avatar { display: grid; flex: 0 0 auto; place-items: center; width: 32px; height: 32px; border-radius: 10px; font-size: 12px; font-weight: 650; }
.avatar--user { color: #4c5870; background: #e9edf5; }
.avatar--ai { color: #fff; background: var(--primary); }
.bubble { max-width: min(76%, 600px); padding: 11px 14px 8px; border-radius: 16px 5px 16px 16px; background: #f2f3f5; }
.bubble p { margin: 0; white-space: pre-wrap; }
.bubble small { display: block; margin-top: 3px; color: #a4a8af; font-size: 10px; text-align: right; }
.answer { min-width: 0; flex: 1; padding-top: 4px; font-size: 15px; line-height: 1.85; }
.thinking { margin: 0; color: var(--text-3); animation: pulse 1.5s infinite; }
.answer__error { padding: 10px 12px; border-radius: 10px; color: var(--danger); background: #fff3f3; }
.caret { display: inline-block; width: 7px; height: 1em; margin-left: 3px; border-radius: 2px; background: var(--primary); animation: pulse 1s infinite; vertical-align: text-bottom; }
.refs { display: grid; gap: 8px; margin-top: 18px; padding-top: 16px; border-top: 1px solid var(--line); }
.refs__title { margin: 0 0 2px; color: var(--text-3); font-size: 12px; }
.ref { display: grid; gap: 7px; width: 100%; padding: 12px; border: 1px solid var(--line); border-radius: 12px; color: inherit; background: #fff; text-align: left; cursor: pointer; }
.ref:hover { border-color: #d4def2; background: #fbfcff; }
.ref__head { display: flex; align-items: center; justify-content: space-between; gap: 10px; }
.ref__text { display: -webkit-box; color: var(--text-2); font-size: 13px; line-height: 1.65; -webkit-box-orient: vertical; -webkit-line-clamp: 2; overflow: hidden; white-space: pre-wrap; }
.ref__text.is-open { display: block; }
.ref small { color: var(--primary); font-size: 11px; }
.composer { position: fixed; z-index: 7; right: max(28px, calc((100vw - var(--sidebar-width) - 920px) / 2)); bottom: 22px; left: max(calc(var(--sidebar-width) + 28px), calc((100vw + var(--sidebar-width) - 920px) / 2)); padding: 15px 16px 10px; border: 1px solid var(--line-strong); border-radius: 22px; background: rgba(255,255,255,.96); box-shadow: var(--shadow-lg); backdrop-filter: blur(20px); }
.composer textarea { display: block; width: 100%; min-height: 34px; max-height: 140px; padding: 0 2px; border: 0; outline: 0; resize: none; color: var(--text); background: transparent; line-height: 1.6; }
.composer textarea::placeholder { color: #a7abb2; }
.composer__tools { display: flex; align-items: center; justify-content: space-between; margin-top: 9px; }
.composer__tools > div { display: flex; gap: 8px; }
.composer__mode { display: flex; align-items: center; gap: 6px; color: var(--text-2); font-size: 12px; }
.tool-button, .send-button { display: grid; place-items: center; width: 34px; height: 34px; padding: 0; border: 0; border-radius: 50%; cursor: pointer; }
.tool-button { color: var(--text-3); background: transparent; }
.tool-button:hover { background: var(--surface-soft); }
.send-button { color: #fff; background: var(--primary); }
.send-button:disabled { color: #b9bdc4; background: #eceef1; cursor: not-allowed; }
.send-button.is-stop { background: var(--text); }
.composer__hint { position: absolute; top: calc(100% + 7px); left: 0; width: 100%; color: #b0b4bb; font-size: 10px; text-align: center; }
@media (max-width: 800px) { .composer { right: 18px; left: calc(var(--sidebar-width) + 18px); } .suggestions { margin-top: 32px; } }
@media (max-width: 560px) { .welcome { justify-content: flex-start; padding-top: 36px; } .welcome__logo { width: 48px; height: 48px; } .mode-switch { width: 100%; } .suggestions button { width: 100%; } .composer { right: 10px; bottom: 14px; left: calc(var(--sidebar-width) + 10px); border-radius: 18px; } .bubble { max-width: 82%; } }
</style>
