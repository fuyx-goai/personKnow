// store.js —— 跨页面共享的状态与用例
//
// 没有引 Pinia：这个规模的应用，一个 reactive 对象加几个 computed 就够了。
// 片段明细（chunks）是"总览"的来源分布和"藏书"页共用的数据源，所以放在这里缓存，
// 摄入/删除之后调 invalidateChunks() 让它失效、下次访问再拉。
import { computed, reactive } from 'vue'
import { api } from './api'

// 一次最多拉多少段。"藏书"页要能翻到全部内容，个人知识库给 1000 足够宽裕。
const CHUNK_LIMIT = 1000

export const state = reactive({
  online: false, // 健康检查是否通过
  config: '', // 给人看的配置摘要
  vectorStore: '', // mem / milvus
  chatModel: '',
  embedModel: '',
  embedAPI: '',

  total: 0, // /api/stats 给出的权威片段总数
  chunks: [], // 已载入的片段明细
  chunksLoading: false,
  chunksLoaded: false,

  toast: null, // { id, text, kind }
})

let toastTimer = null

// notify 顶部提示条：kind 为 'bad' 时留久一点，让人看清错误
export function notify(text, kind = 'ok') {
  state.toast = { id: Date.now(), text, kind }
  clearTimeout(toastTimer)
  toastTimer = setTimeout(() => {
    state.toast = null
  }, kind === 'bad' ? 5200 : 3000)
}

// bootstrap 应用挂载时跑一次
export async function bootstrap() {
  try {
    await api.health()
    state.online = true
  } catch {
    state.online = false
    notify('网关没有响应，界面上的数据可能是旧的', 'bad')
  }

  await Promise.all([loadConfig(), refreshStats()])
  // 片段列表失败不阻塞首屏，交给需要它的页面自己提示
  ensureChunks().catch(() => {})
}

export async function loadConfig() {
  try {
    const c = await api.config()
    state.config = c.config || ''
    state.vectorStore = c.vectorStore || ''
    state.chatModel = c.chatModel || ''
    state.embedModel = c.embedModel || ''
    state.embedAPI = c.embedAPI || ''
  } catch (e) {
    notify(`读取配置失败：${e.message}`, 'bad')
  }
}

export async function refreshStats() {
  try {
    const s = await api.stats()
    state.total = s.chunks ?? 0
    state.vectorStore = s.vectorStore || state.vectorStore
    state.embedAPI = s.embedAPI || state.embedAPI
  } catch (e) {
    notify(`读取统计失败：${e.message}`, 'bad')
  }
}

export async function ensureChunks(force = false) {
  if (state.chunksLoaded && !force) return state.chunks

  state.chunksLoading = true
  try {
    const r = await api.chunks(CHUNK_LIMIT)
    state.chunks = r?.chunks || []
    state.chunksLoaded = true
    return state.chunks
  } finally {
    state.chunksLoading = false
  }
}

// invalidateChunks 摄入或删除之后调用：丢掉缓存，等下次访问重新拉
export function invalidateChunks() {
  state.chunksLoaded = false
  state.chunks = []
}

// shelves 按来源聚合，总览的"来源分布"和藏书页的分组都用它
export const shelves = computed(() => {
  const groups = new Map()

  for (const chunk of state.chunks) {
    let g = groups.get(chunk.source)
    if (!g) {
      g = { source: chunk.source, count: 0, bytes: 0, items: [] }
      groups.set(chunk.source, g)
    }
    g.count += 1
    g.bytes += chunk.bytes || 0
    g.items.push(chunk)
  }

  return [...groups.values()].sort(
    (a, b) => b.count - a.count || a.source.localeCompare(b.source),
  )
})

// 载入是否被上限截断（总览会据此提示一句）
export const truncated = computed(
  () => state.chunksLoaded && state.total > state.chunks.length,
)
