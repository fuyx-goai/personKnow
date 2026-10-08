import { createAuthStore } from './utils/auth'

const JSON_HEADERS = { 'Content-Type': 'application/json' }

export class ApiError extends Error {
  constructor(payload = {}, status = 0) {
    super(payload.message || payload.error || `请求失败（HTTP ${status}）`)
    this.name = 'ApiError'
    this.code = payload.code || 'REQUEST_FAILED'
    this.requestId = payload.request_id || ''
    this.status = status
  }
}

async function responsePayload(response) {
  const text = await response.text()
  if (!text) return null
  try { return JSON.parse(text) } catch { return { message: text } }
}

function query(values = {}) {
  const params = new URLSearchParams()
  Object.entries(values).forEach(([key, value]) => {
    if (value !== '' && value !== undefined && value !== null) params.set(key, value)
  })
  const result = params.toString()
  return result ? `?${result}` : ''
}

export function createAPIClient(options = {}) {
  const fetcher = options.fetcher || fetch
  const auth = options.auth
  const refresh = options.refresh

  async function request(path, requestOptions = {}, retried = false) {
    const token = auth?.accessToken?.()
    let response
    try {
      response = await fetcher(path, {
        ...requestOptions,
        headers: {
          ...(requestOptions.body instanceof FormData ? {} : JSON_HEADERS),
          ...(token ? { Authorization: `Bearer ${token}` } : {}),
          ...(requestOptions.headers || {}),
        },
      })
    } catch (error) {
      if (error.name === 'AbortError') throw error
      throw new ApiError({ code: 'NETWORK_ERROR', message: '连不上知识库服务，请检查网关状态' })
    }
    if (response.status === 401 && !retried && refresh && auth?.refreshToken?.()) {
      auth.accept(await refresh(auth.refreshToken()))
      return request(path, requestOptions, true)
    }
    const payload = await responsePayload(response)
    if (!response.ok) throw new ApiError(payload || {}, response.status)
    return payload
  }

  return {
    auth,
    request,
    health: () => request('/api/health'),
    config: () => request('/api/config'),
    me: () => request('/api/v1/me'),
    logout: () => request('/api/v1/auth/logout', { method: 'POST' }),
    createTicket: () => request('/api/v1/auth/web/tickets', { method: 'POST' }),
    pollTicket: (ticket) => request(`/api/v1/auth/web/tickets/${ticket.id}`, {
      headers: { 'X-Login-Ticket-Secret': ticket.secret, 'X-Device-Label': 'Web 浏览器' },
    }),
    libraries: (filter = {}) => request(`/api/v1/libraries${query(filter)}`),
    createLibrary: (data) => request('/api/v1/libraries', { method: 'POST', body: JSON.stringify(data) }),
    updateLibrary: (id, data) => request(`/api/v1/libraries/${id}`, { method: 'PATCH', body: JSON.stringify(data) }),
    deleteLibrary: (id) => request(`/api/v1/libraries/${id}`, { method: 'DELETE' }),
    librarySettings: (id) => request(`/api/v1/libraries/${id}/retrieval-settings`),
    updateLibrarySettings: (id, data) => request(`/api/v1/libraries/${id}/retrieval-settings`, { method: 'PATCH', body: JSON.stringify(data) }),
    documents: (filter = {}) => request(`/api/v1/documents${query(filter)}`),
    uploadDocument: (libraryId, file) => {
      const body = new FormData()
      body.append('library_id', libraryId)
      body.append('file', file)
      return request('/api/v1/documents', { method: 'POST', body })
    },
    documentContent: (id) => request(`/api/v1/documents/${id}/content`),
    updateDocument: (id, data) => request(`/api/v1/documents/${id}`, { method: 'PATCH', body: JSON.stringify(data) }),
    editDocument: (id, text) => request(`/api/v1/documents/${id}/content`, { method: 'PATCH', body: JSON.stringify({ text }) }),
    reindexDocument: (id) => request(`/api/v1/documents/${id}/reindex`, { method: 'POST' }),
    deleteDocument: (id) => request(`/api/v1/documents/${id}`, { method: 'DELETE' }),
    indexJob: (id) => request(`/api/v1/index-jobs/${id}`),
    chatSessions: () => request('/api/v1/chat/sessions'),
    createChatSession: (data) => request('/api/v1/chat/sessions', { method: 'POST', body: JSON.stringify(data) }),
    chatHistory: (id) => request(`/api/v1/chat/sessions/${id}`),
    deleteChatSession: (id) => request(`/api/v1/chat/sessions/${id}`, { method: 'DELETE' }),
    usageSummary: () => request('/api/v1/usage/summary'),
    sessions: () => request('/api/v1/me/sessions'),
    auditLogs: () => request('/api/v1/audit-logs?limit=30'),
  }
}

export const auth = createAuthStore()

async function refreshSession(refreshToken) {
  const response = await fetch('/api/v1/auth/refresh', {
    method: 'POST', headers: JSON_HEADERS, body: JSON.stringify({ refresh_token: refreshToken }),
  })
  const payload = await responsePayload(response)
  if (!response.ok) throw new ApiError(payload || {}, response.status)
  return payload
}

export const api = createAPIClient({ auth, refresh: refreshSession })

export async function streamChat(sessionId, question, handlers = {}, signal) {
  let response
  try {
    response = await fetch(`/api/v1/chat/sessions/${sessionId}/messages/stream`, {
      method: 'POST', headers: { ...JSON_HEADERS, Authorization: `Bearer ${auth.accessToken()}` },
      body: JSON.stringify({ question }), signal,
    })
  } catch (error) {
    if (error.name === 'AbortError') return
    throw new ApiError({ code: 'NETWORK_ERROR', message: '问答连接失败' })
  }
  if (!response.ok) throw new ApiError(await responsePayload(response), response.status)
  await consumeSSE(response.body, handlers)
}

async function consumeSSE(body, handlers) {
  const reader = body.getReader()
  const decoder = new TextDecoder()
  let buffer = ''
  for (;;) {
    const { done, value } = await reader.read()
    if (done) break
    buffer += decoder.decode(value, { stream: true }).replace(/\r\n/g, '\n')
    let boundary = buffer.indexOf('\n\n')
    while (boundary >= 0) {
      emitSSE(buffer.slice(0, boundary), handlers)
      buffer = buffer.slice(boundary + 2)
      boundary = buffer.indexOf('\n\n')
    }
  }
}

function emitSSE(block, handlers) {
  const lines = block.split('\n')
  const type = lines.find((line) => line.startsWith('event:'))?.slice(6).trim()
  const raw = lines.filter((line) => line.startsWith('data:')).map((line) => line.slice(5).trim()).join('\n')
  if (!raw) return
  const data = JSON.parse(raw)
  if (type === 'delta') handlers.onDelta?.(data.delta)
  else if (type === 'reference') handlers.onReference?.(data)
  else if (type === 'usage') handlers.onUsage?.(data)
  else if (type === 'error') handlers.onError?.(data)
  else if (type === 'done') handlers.onDone?.(data)
}

export async function pollIndexJob(jobId, options = {}) {
  const sleep = options.sleep || ((duration) => new Promise((resolve) => setTimeout(resolve, duration)))
  for (let attempt = 0; attempt < (options.maxAttempts || 120); attempt += 1) {
    const job = await api.indexJob(jobId)
    if (job.status === 'ready' || job.status === 'failed') return job
    await sleep(options.interval || 1200)
  }
  throw new ApiError({ code: 'INDEX_JOB_TIMEOUT', message: '索引任务等待超时' })
}
