// api.js —— 唯一的接口访问出口
//
// 页面组件一律不直接写 fetch：地址、错误文案、SSE 解析都收在这里，
// 后端调整接口时只需要动这一个文件。
//
// 后端契约（见 internal/gateway/handler）：
//   GET    /api/health         -> {status:"ok"}
//   GET    /api/config         -> {config, vectorStore, chatModel, embedModel, embedAPI}
//   GET    /api/stats          -> {vectorStore, chunks, embedAPI}
//   GET    /api/chunks?limit=  -> {chunks:[{id,source,content,bytes}], count}
//   DELETE /api/chunks?source= -> {deleted, source}
//   POST   /api/ingest         <- {path}          -> {totalChunks, results:[{file,chunks,error,success}]}
//   POST   /api/chat           <- {question}      -> {answer, references:[{source,score,snippet}]}
//   POST   /api/chat/stream    <- {question}      -> SSE

const JSON_HEADERS = { 'Content-Type': 'application/json' }

// request 统一处理：非 2xx 时后端返回 {"error":"..."}，把它原样抛出来当提示文案
async function request(path, options = {}) {
  let resp
  try {
    resp = await fetch(path, options)
  } catch {
    throw new Error('连不上服务，确认网关进程还在跑？')
  }

  const text = await resp.text()
  if (!resp.ok) {
    throw new Error(extractError(text, resp.status))
  }
  return text ? JSON.parse(text) : null
}

// extractError 尽量把后端的中文错误取出来，取不到就退回状态码
function extractError(text, status) {
  const fallback = text || `HTTP ${status}`
  try {
    const data = JSON.parse(text)
    return data?.error || fallback
  } catch {
    return fallback
  }
}

export const api = {
  health: () => request('/api/health'),
  config: () => request('/api/config'),
  stats: () => request('/api/stats'),

  chunks: (limit = 1000) => request(`/api/chunks?limit=${limit}`),

  // removeChunks 不传 source 即清空整库（后端约定：空 source = 全删）
  removeChunks: (source) =>
    request(`/api/chunks${source ? `?source=${encodeURIComponent(source)}` : ''}`, {
      method: 'DELETE',
    }),

  ingest: (path) =>
    request('/api/ingest', {
      method: 'POST',
      headers: JSON_HEADERS,
      body: JSON.stringify({ path }),
    }),

  ask: (question) =>
    request('/api/chat', {
      method: 'POST',
      headers: JSON_HEADERS,
      body: JSON.stringify({ question }),
    }),

  askStream: (question, handlers, signal) => askStream(question, handlers, signal),
}

// askStream 消费 /api/chat/stream 推来的 SSE
//
// 事件形状（见 handler.ChatStream）：
//   data: {"delta":"..."}                    增量文本，收到就追加
//   data: {"references":[...],"done":true}   收尾，附带引用来源
//   data: [DONE]                             流结束标记
async function askStream(question, { onDelta, onReferences } = {}, signal) {
  let resp
  try {
    resp = await fetch('/api/chat/stream', {
      method: 'POST',
      headers: JSON_HEADERS,
      body: JSON.stringify({ question }),
      signal,
    })
  } catch (e) {
    if (e.name === 'AbortError') return // 用户点了"停止"，不算错误
    throw new Error('连不上服务，确认网关进程还在跑？')
  }

  if (!resp.ok) {
    throw new Error(extractError(await resp.text(), resp.status))
  }

  const reader = resp.body.getReader()
  const decoder = new TextDecoder()
  let buffer = ''

  for (;;) {
    const { done, value } = await reader.read()
    if (done) break

    // stream:true —— 一个汉字可能被切成两个 chunk，交给解码器攒
    buffer += decoder.decode(value, { stream: true })
    buffer = buffer.replace(/\r\n/g, '\n')

    let cut
    while ((cut = buffer.indexOf('\n\n')) >= 0) {
      const block = buffer.slice(0, cut)
      buffer = buffer.slice(cut + 2)

      for (const line of block.split('\n')) {
        if (!line.startsWith('data:')) continue
        const payload = line.slice(5).trim()
        if (!payload) continue
        if (payload === '[DONE]') return

        let evt
        try {
          evt = JSON.parse(payload)
        } catch {
          continue // 半截 JSON（理论上不该有）直接跳过，不打断整条流
        }
        if (evt.error) throw new Error(evt.error)
        if (evt.delta) onDelta?.(evt.delta)
        if (evt.references) onReferences?.(evt.references)
      }
    }
  }
}
