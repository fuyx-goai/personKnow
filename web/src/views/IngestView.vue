<script setup>
import { computed, ref } from 'vue'
import { api } from '../api'
import { ensureChunks, invalidateChunks, notify, refreshStats } from '../store'
import AppIcon from '../components/AppIcon.vue'

const PRESETS = ['./docs', './README.md']
const path = ref('./docs')
const busy = ref(false)
const summary = ref(null)
const failure = ref('')
const okFiles = computed(() => summary.value?.results?.filter((item) => item.success) ?? [])
const badFiles = computed(() => summary.value?.results?.filter((item) => !item.success) ?? [])

async function submit() {
  const target = path.value.trim()
  if (!target || busy.value) return
  busy.value = true
  failure.value = ''
  summary.value = null
  try {
    summary.value = await api.ingest(target)
    invalidateChunks()
    await Promise.all([refreshStats(), ensureChunks().catch(() => {})])
    notify(summary.value.totalChunks > 0 ? `已入库 ${summary.value.totalChunks} 段` : '这次没有新增片段，请查看明细', summary.value.totalChunks > 0 ? 'ok' : 'bad')
  } catch (error) {
    failure.value = error.message
    notify(`摄入失败：${error.message}`, 'bad')
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <section class="view ingest">
    <header class="page-head">
      <span class="page-head__icon"><AppIcon name="upload" :size="24" /></span>
      <div><h1>添加知识资料</h1><p>读取本地文件或目录，自动切分内容并建立语义索引。</p></div>
    </header>

    <form class="ingest-card" @submit.prevent="submit">
      <div class="drop-visual">
        <span><AppIcon name="file" :size="30" /></span>
        <strong>从服务端路径导入</strong>
        <p>支持单个文件或整个目录，重复导入会更新同一来源。</p>
      </div>

      <label class="label" for="ingest-path">文件或目录路径</label>
      <div class="path-field">
        <AppIcon name="search" :size="18" />
        <input id="ingest-path" v-model="path" type="text" spellcheck="false" autocomplete="off" placeholder="./docs" />
      </div>

      <div class="presets">
        <span>快速选择</span>
        <button v-for="preset in PRESETS" :key="preset" type="button" :disabled="busy" @click="path = preset">{{ preset }}</button>
      </div>

      <div class="submit-row">
        <p><AppIcon name="database" :size="16" />相对路径以服务端工作目录为准</p>
        <button class="btn btn--primary" type="submit" :disabled="busy || !path.trim()">
          <AppIcon :name="busy ? 'refresh' : 'upload'" :size="17" />{{ busy ? '正在处理…' : '开始导入' }}
        </button>
      </div>
    </form>

    <p v-if="failure" class="alert">{{ failure }}</p>

    <section v-if="summary" class="result">
      <header class="result__head">
        <div><h2>导入结果</h2><p>共入库 {{ summary.totalChunks }} 段内容</p></div>
        <span>{{ okFiles.length }} 成功<template v-if="badFiles.length"> · {{ badFiles.length }} 失败</template></span>
      </header>
      <div class="rows">
        <div v-for="(item, index) in summary.results" :key="item.file" class="row" :class="item.success ? 'is-ok' : 'is-bad'" :style="{ animationDelay: `${Math.min(index, 14) * 30}ms` }">
          <span class="row__icon"><AppIcon :name="item.success ? 'file' : 'more'" :size="18" /></span>
          <span class="row__file">{{ item.file }}</span>
          <span v-if="item.success" class="row__tag">{{ item.chunks }} 段</span>
          <span v-else class="row__msg">{{ item.error || '未知错误' }}</span>
        </div>
      </div>
    </section>
  </section>
</template>

<style scoped>
.ingest { max-width: 900px; padding-top: 46px; }
.page-head { display: flex; align-items: center; gap: 16px; margin-bottom: 26px; }
.page-head__icon { display: grid; place-items: center; width: 48px; height: 48px; border-radius: 15px; color: var(--primary); background: var(--primary-soft); }
.page-head h1 { margin: 0; font-size: 28px; letter-spacing: -.03em; }
.page-head p { margin: 4px 0 0; color: var(--text-3); font-size: 13px; }
.ingest-card { padding: 28px; border: 1px solid var(--line); border-radius: 22px; background: #fff; box-shadow: var(--shadow-sm); }
.drop-visual { display: flex; align-items: center; margin-bottom: 26px; padding: 20px; border-radius: 16px; background: linear-gradient(135deg, #f3f7ff, #fafcff); }
.drop-visual > span { display: grid; flex: 0 0 auto; place-items: center; width: 52px; height: 52px; margin-right: 14px; border-radius: 15px; color: #fff; background: var(--primary); }
.drop-visual strong { font-size: 15px; }
.drop-visual p { margin: 2px 0 0 auto; color: var(--text-3); font-size: 12px; }
.path-field { display: flex; align-items: center; gap: 10px; height: 48px; padding: 0 14px; border: 1px solid var(--line-strong); border-radius: 13px; color: var(--text-3); }
.path-field:focus-within { border-color: var(--primary); box-shadow: 0 0 0 3px rgba(23,105,255,.1); }
.path-field input { flex: 1; min-width: 0; border: 0; outline: 0; color: var(--text); background: transparent; font-family: var(--mono); font-size: 13px; }
.presets { display: flex; align-items: center; gap: 8px; margin-top: 12px; }
.presets span { margin-right: 2px; color: var(--text-3); font-size: 11px; }
.presets button { padding: 5px 9px; border: 0; border-radius: 8px; color: var(--text-2); background: var(--surface-soft); font-family: var(--mono); font-size: 11px; cursor: pointer; }
.presets button:hover { color: var(--primary); background: var(--primary-soft); }
.submit-row { display: flex; align-items: center; justify-content: space-between; gap: 16px; margin-top: 28px; padding-top: 20px; border-top: 1px solid var(--line); }
.submit-row p { display: flex; align-items: center; gap: 7px; margin: 0; color: var(--text-3); font-size: 11px; }
.alert { padding: 12px 14px; border-radius: 12px; color: var(--danger); background: #fff3f3; font-size: 13px; }
.result { margin-top: 28px; }
.result__head { display: flex; align-items: flex-end; justify-content: space-between; margin-bottom: 12px; }
.result__head h2 { margin: 0; font-size: 18px; }
.result__head p { margin: 3px 0 0; color: var(--text-3); font-size: 12px; }
.result__head > span { color: var(--text-3); font-size: 12px; }
.rows { display: grid; gap: 8px; }
.row { display: grid; grid-template-columns: 34px minmax(0,1fr) auto; align-items: center; gap: 10px; padding: 12px 14px; border: 1px solid var(--line); border-radius: 13px; background: #fff; animation: rise .4s var(--ease) backwards; }
.row__icon { display: grid; place-items: center; width: 34px; height: 34px; border-radius: 10px; color: var(--success); background: #ecf8f2; }
.row.is-bad .row__icon { color: var(--danger); background: #fff1f1; }
.row__file { overflow: hidden; font-family: var(--mono); font-size: 12px; text-overflow: ellipsis; white-space: nowrap; }
.row__tag { color: var(--success); font-size: 11px; }
.row__msg { color: var(--danger); font-size: 11px; }
@media (max-width: 640px) { .drop-visual { align-items: flex-start; } .drop-visual p { display: none; } .submit-row { align-items: stretch; flex-direction: column; } .submit-row .btn { width: 100%; } }
</style>
