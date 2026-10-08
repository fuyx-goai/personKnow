<script setup>
import { computed, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { ensureChunks, refreshStats, shelves, state, truncated } from '../store'
import { bytes } from '../utils/format'
import AppIcon from '../components/AppIcon.vue'

const router = useRouter()
const sourceCount = computed(() => shelves.value.length)
const busiest = computed(() => shelves.value[0] || null)
const maxCount = computed(() => Math.max(1, ...shelves.value.map((item) => item.count)))
const storeLabel = computed(() => state.vectorStore === 'milvus' ? 'Milvus' : '本地内存')
const embedLabel = computed(() => ({ openai: 'OpenAI', ark_multimodal: '方舟多模态' })[state.embedAPI] || state.embedAPI || '未配置')
const percentOf = (count) => `${Math.round((count / maxCount.value) * 100)}%`

onMounted(() => { refreshStats(); ensureChunks().catch(() => {}) })
function openSource(source) { router.push({ path: '/library', query: { source } }) }
</script>

<template>
  <section class="view overview">
    <header class="hero">
      <div>
        <span class="eyebrow"><AppIcon name="sparkle" :size="15" />KNOWLEDGE SPACE</span>
        <h1>你的知识，现在一目了然</h1>
        <p>管理资料、追踪知识结构，并从已有内容中快速获得可靠答案。</p>
      </div>
      <div class="hero__actions">
        <RouterLink class="btn btn--ghost" to="/ingest"><AppIcon name="upload" :size="17" />添加资料</RouterLink>
        <RouterLink class="btn btn--primary" to="/ask"><AppIcon name="chat" :size="17" />开始提问</RouterLink>
      </div>
    </header>

    <div class="stats">
      <article class="stat stat--primary">
        <span class="stat__icon"><AppIcon name="database" :size="22" /></span>
        <div><small>知识片段</small><strong>{{ state.total }}</strong><p>已建立语义索引</p></div>
      </article>
      <article class="stat">
        <span class="stat__icon"><AppIcon name="file" :size="22" /></span>
        <div><small>资料来源</small><strong>{{ sourceCount }}</strong><p>{{ busiest ? `最多：${busiest.source}` : '等待添加资料' }}</p></div>
      </article>
      <article class="stat">
        <span class="stat__icon"><AppIcon name="library" :size="22" /></span>
        <div><small>向量存储</small><strong class="stat__text">{{ storeLabel }}</strong><p>{{ state.vectorStore || 'mem' }}</p></div>
      </article>
      <article class="stat">
        <span class="stat__icon"><AppIcon name="sparkle" :size="22" /></span>
        <div><small>向量接口</small><strong class="stat__text">{{ embedLabel }}</strong><p>{{ state.embedModel || '等待配置' }}</p></div>
      </article>
    </div>

    <section class="distribution">
      <header class="distribution__head">
        <div><h2>资料分布</h2><p>按来源查看当前知识库的内容组成</p></div>
        <span v-if="truncated">已显示 {{ state.chunks.length }} / {{ state.total }} 段</span>
      </header>

      <div v-if="state.chunksLoading" class="empty">正在读取知识库…</div>
      <div v-else-if="!shelves.length" class="empty">
        <span class="empty__mark">○</span><p class="empty__text">知识库还是空的，添加第一份资料开始构建吧。</p>
        <RouterLink class="btn btn--primary" to="/ingest">添加资料</RouterLink>
      </div>
      <div v-else class="sources">
        <button v-for="(item, index) in shelves" :key="item.source" class="source" type="button" @click="openSource(item.source)">
          <span class="source__icon"><AppIcon name="file" :size="18" /></span>
          <span class="source__copy"><strong>{{ item.source }}</strong><small>{{ item.count }} 个片段 · {{ bytes(item.bytes) }}</small></span>
          <span class="source__bar"><i :style="{ width: percentOf(item.count), animationDelay: `${index * 45}ms` }" /></span>
          <span class="source__open">查看 ›</span>
        </button>
      </div>
    </section>
  </section>
</template>

<style scoped>
.overview { padding-top: 46px; }
.hero { display: flex; align-items: flex-end; justify-content: space-between; gap: 28px; }
.eyebrow { display: flex; align-items: center; gap: 7px; margin-bottom: 12px; color: var(--primary); font-size: 11px; font-weight: 700; letter-spacing: .12em; }
.hero h1 { margin: 0; font-size: clamp(30px, 4vw, 44px); line-height: 1.18; letter-spacing: -.04em; }
.hero p { margin: 12px 0 0; color: var(--text-3); }
.hero__actions { display: flex; flex: 0 0 auto; gap: 10px; }
.stats { display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); gap: 14px; margin-top: 36px; }
.stat { display: flex; min-width: 0; gap: 14px; padding: 20px; border: 1px solid var(--line); border-radius: 18px; background: #fff; box-shadow: var(--shadow-sm); }
.stat--primary { border-color: #dce7ff; background: linear-gradient(145deg, #f7faff, #fff); }
.stat__icon { display: grid; flex: 0 0 auto; place-items: center; width: 42px; height: 42px; border-radius: 13px; color: var(--primary); background: var(--primary-soft); }
.stat > div { min-width: 0; }
.stat small { display: block; color: var(--text-3); font-size: 11px; }
.stat strong { display: block; margin-top: 3px; font-size: 28px; line-height: 1.25; }
.stat strong.stat__text { overflow: hidden; padding-top: 5px; font-size: 17px; text-overflow: ellipsis; white-space: nowrap; }
.stat p { overflow: hidden; margin: 4px 0 0; color: var(--text-3); font-size: 11px; text-overflow: ellipsis; white-space: nowrap; }
.distribution { margin-top: 24px; padding: 24px; border: 1px solid var(--line); border-radius: 20px; background: #fff; box-shadow: var(--shadow-sm); }
.distribution__head { display: flex; align-items: flex-start; justify-content: space-between; gap: 16px; margin-bottom: 16px; }
.distribution__head h2 { margin: 0; font-size: 17px; }
.distribution__head p { margin: 4px 0 0; color: var(--text-3); font-size: 12px; }
.distribution__head > span { color: var(--text-3); font-size: 11px; }
.sources { display: grid; }
.source { display: grid; grid-template-columns: 36px minmax(160px, 1.4fr) minmax(120px, 2fr) auto; align-items: center; gap: 14px; width: 100%; padding: 13px 8px; border: 0; border-top: 1px solid var(--line); color: inherit; background: transparent; text-align: left; cursor: pointer; }
.source:hover { background: #fafbfc; }
.source__icon { display: grid; place-items: center; width: 36px; height: 36px; border-radius: 11px; color: #627ba7; background: #edf2fa; }
.source__copy { min-width: 0; }
.source__copy strong, .source__copy small { display: block; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.source__copy strong { font-size: 13px; font-weight: 600; }
.source__copy small { color: var(--text-3); font-size: 11px; }
.source__bar { height: 6px; border-radius: 99px; background: #eef0f3; overflow: hidden; }
.source__bar i { display: block; height: 100%; border-radius: inherit; background: linear-gradient(90deg, #5b8fff, var(--primary)); transform-origin: left; animation: grow .7s var(--ease) backwards; }
.source__open { color: var(--text-3); font-size: 11px; }
@keyframes grow { from { transform: scaleX(0); } to { transform: scaleX(1); } }
@media (max-width: 1000px) { .stats { grid-template-columns: repeat(2, minmax(0, 1fr)); } }
@media (max-width: 680px) { .hero { align-items: flex-start; flex-direction: column; } .stats { grid-template-columns: 1fr; } .source { grid-template-columns: 36px 1fr auto; } .source__bar { display: none; } }
@media (max-width: 480px) { .hero__actions { width: 100%; } .hero__actions .btn { flex: 1; } .distribution { padding: 17px; } }
</style>
