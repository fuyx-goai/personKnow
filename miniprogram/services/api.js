const { createAuthSession } = require('./auth')
const { createSSEParser, createUtf8Decoder } = require('../utils/sse')

class ApiError extends Error {
  constructor(payload = {}, statusCode = 0) {
    const normalized = typeof payload === 'string' ? { message: payload } : payload || {}
    const isLegacyGateway = statusCode === 404 && /404\s+page\s+not\s+found/i.test(normalized.message || '')
    super(isLegacyGateway ? '当前 Go 网关未加载小程序接口，请重启最新 Go 网关' : normalized.message || normalized.error || `请求失败（HTTP ${statusCode}）`)
    this.name = 'ApiError'
    this.code = isLegacyGateway ? 'API_VERSION_MISMATCH' : normalized.code || 'REQUEST_FAILED'
    this.requestId = normalized.request_id || ''
    this.statusCode = statusCode
  }
}

// 统一去掉服务地址末尾的斜杠，避免配置成 http://host/ 后拼出 //api 的无效路径。
function normalizeBaseUrl(value) {
  return String(value || '').trim().replace(/\/+$/, '')
}

// 所有小程序请求都经过同一个 URL 拼接点，保证登录、上传和流式问答使用同一基址。
function buildURL(baseUrl, path) {
  return `${normalizeBaseUrl(baseUrl)}/${String(path || '').replace(/^\/+/, '')}`
}

// 日志只保留排障需要的元数据；调用方不得把 token、微信 code 或请求正文放入 fields。
function logAPI(logger, event, fields = {}) {
  logger?.info?.(`[miniprogram-api] ${event}`, { event, ...fields })
}

function requestID(response = {}) {
  const headers = response.header || response.headers || {}
  return headers['X-Request-ID'] || headers['x-request-id'] || ''
}

function appBaseUrl() {
  return normalizeBaseUrl(getApp().globalData.apiBaseUrl || 'http://127.0.0.1:8080')
}

function wxTransport(options) {
  return new Promise((resolve, reject) => {
    wx.request({ ...options, success: resolve, fail: reject })
  })
}

function wxLogin() {
  return new Promise((resolve, reject) => wx.login({ success: resolve, fail: reject }))
}

function queryString(values = {}) {
  const parts = Object.entries(values)
    .filter(([, value]) => value !== '' && value !== undefined && value !== null)
    .map(([key, value]) => `${encodeURIComponent(key)}=${encodeURIComponent(value)}`)
  return parts.length ? `?${parts.join('&')}` : ''
}

function createEndpoints(request, auth) {
  return {
    auth, request,
    health: () => request('/api/health', { skipRefresh: true }),
    status: () => request('/api/v1/status', { skipRefresh: true }),
    config: () => request('/api/config', { skipRefresh: true }),
    me: () => request('/api/v1/me'),
    confirmWebTicket: (id, secret) => request(`/api/v1/auth/web/tickets/${id}/confirm`, { method: 'POST', data: { secret } }),
    libraries: (filter = {}) => request(`/api/v1/libraries${queryString(filter)}`),
    createLibrary: (data) => request('/api/v1/libraries', { method: 'POST', data }),
    updateLibrary: (id, data) => request(`/api/v1/libraries/${id}`, { method: 'PATCH', data }),
    deleteLibrary: (id) => request(`/api/v1/libraries/${id}`, { method: 'DELETE' }),
    librarySettings: (id) => request(`/api/v1/libraries/${id}/retrieval-settings`),
    updateLibrarySettings: (id, data) => request(`/api/v1/libraries/${id}/retrieval-settings`, { method: 'PATCH', data }),
    reindexLibrary: (id) => request(`/api/v1/libraries/${id}/reindex`, { method: 'POST' }),
    documents: (filter = {}) => request(`/api/v1/documents${queryString(filter)}`),
    document: (id) => request(`/api/v1/documents/${id}`),
    documentContent: (id) => request(`/api/v1/documents/${id}/content`),
    updateDocument: (id, data) => request(`/api/v1/documents/${id}`, { method: 'PATCH', data }),
    editDocument: (id, text) => request(`/api/v1/documents/${id}/content`, { method: 'PATCH', data: { text } }),
    reindexDocument: (id) => request(`/api/v1/documents/${id}/reindex`, { method: 'POST' }),
    deleteDocument: (id) => request(`/api/v1/documents/${id}`, { method: 'DELETE' }),
    indexJob: (id) => request(`/api/v1/index-jobs/${id}`),
    retryIndexJob: (id) => request(`/api/v1/index-jobs/${id}/retry`, { method: 'POST' }),
    chatSessions: () => request('/api/v1/chat/sessions'),
    createChatSession: (data) => request('/api/v1/chat/sessions', { method: 'POST', data }),
    chatHistory: (id) => request(`/api/v1/chat/sessions/${id}`),
    deleteChatSession: (id) => request(`/api/v1/chat/sessions/${id}`, { method: 'DELETE' }),
    usageSummary: () => request('/api/v1/usage/summary'),
    auditLogs: () => request('/api/v1/audit-logs?limit=20'),
  }
}

function createAPIClient(options = {}) {
  const transport = options.transport || wxTransport
  const baseUrl = options.baseUrl || appBaseUrl
  const auth = options.auth
  const logger = options.logger || console

  async function request(path, requestOptions = {}, retried = false) {
    const token = auth?.accessToken?.()
    const method = requestOptions.method || 'GET'
    const startedAt = Date.now()
    logAPI(logger, 'request_started', { method, path, retried })
    let response
    try {
      response = await transport({
        url: buildURL(baseUrl(), path), method, data: requestOptions.data,
        timeout: requestOptions.timeout || 30000,
        header: {
          'Content-Type': 'application/json',
          ...(token ? { Authorization: `Bearer ${token}` } : {}),
          ...(requestOptions.header || {}),
        },
      })
    } catch (error) {
      logAPI(logger, 'request_failed', { method, path, duration_ms: Date.now() - startedAt, error: error.errMsg || error.message || String(error) })
      throw error
    }
    logAPI(logger, 'request_finished', {
      method, path, status_code: response.statusCode,
      duration_ms: Date.now() - startedAt, request_id: requestID(response),
    })
    // 401 只允许一次刷新重试，避免失效会话在页面请求链路中无限递归。
    if (response.statusCode === 401 && auth?.refresh && !retried && !requestOptions.skipRefresh) {
      await auth.refresh()
      return request(path, requestOptions, true)
    }
    if (response.statusCode < 200 || response.statusCode >= 300) {
      throw new ApiError(response.data, response.statusCode)
    }
    return response.data
  }

  return createEndpoints(request, auth)
}

async function pollIndexJob(fetchJob, options = {}) {
  const sleep = options.sleep || ((duration) => new Promise((resolve) => setTimeout(resolve, duration)))
  const interval = options.interval || 1200
  const maxAttempts = options.maxAttempts || 120
  for (let attempt = 0; attempt < maxAttempts; attempt += 1) {
    const job = await fetchJob()
    if (job.status === 'ready' || job.status === 'failed') return job
    await sleep(interval)
  }
  throw new ApiError({ code: 'INDEX_JOB_TIMEOUT', message: '索引任务等待超时' })
}

// 鉴权接口单独抽出，既能复用统一错误处理，也能在没有微信运行时的 Node 测试中验证真实 URL。
function createAuthAPI(options = {}) {
  const transport = options.transport || wxTransport
  const baseUrl = options.baseUrl || appBaseUrl
  const logger = options.logger || console

  async function rawRequest(path, requestOptions = {}) {
    const method = requestOptions.method || 'POST'
    const startedAt = Date.now()
    logAPI(logger, 'request_started', { method, path, auth_request: true })
    try {
      const response = await transport({
        url: buildURL(baseUrl(), path), method, data: requestOptions.data,
        header: { 'Content-Type': 'application/json', ...(requestOptions.header || {}) },
        timeout: requestOptions.timeout || 30000,
      })
      logAPI(logger, 'request_finished', {
        method, path, status_code: response.statusCode,
        duration_ms: Date.now() - startedAt, request_id: requestID(response),
      })
      if (response.statusCode < 200 || response.statusCode >= 300) {
        throw new ApiError(response.data, response.statusCode)
      }
      return response.data
    } catch (error) {
      if (!(error instanceof ApiError)) {
        logAPI(logger, 'request_failed', { method, path, duration_ms: Date.now() - startedAt, error: error.errMsg || error.message || String(error) })
      }
      throw error
    }
  }

  return {
    exchange: (data) => rawRequest('/api/v1/auth/wechat/login', { data }),
    refresh: (refreshToken) => rawRequest('/api/v1/auth/refresh', { data: { refresh_token: refreshToken } }),
    revoke: (token) => rawRequest('/api/v1/auth/logout', { header: { Authorization: `Bearer ${token}` } }),
  }
}

const authAPI = createAuthAPI()
const auth = createAuthSession({
  login: wxLogin,
  exchange: authAPI.exchange,
  refresh: authAPI.refresh,
  revoke: authAPI.revoke,
})
const client = createAPIClient({ auth })

function askStream(sessionID, question, handlers = {}) {
  const decoder = createUtf8Decoder()
  const parser = createSSEParser(handlers)
  const path = `/api/v1/chat/sessions/${sessionID}/messages/stream`
  const startedAt = Date.now()
  logAPI(console, 'stream_started', { method: 'POST', path })
  const task = wx.request({
    url: buildURL(appBaseUrl(), path),
    method: 'POST',
    data: { question },
    header: { 'Content-Type': 'application/json', Authorization: `Bearer ${auth.accessToken()}` },
    enableChunked: true,
    timeout: 120000,
    success(response) {
      logAPI(console, 'stream_finished', {
        method: 'POST', path, status_code: response.statusCode,
        duration_ms: Date.now() - startedAt, request_id: requestID(response),
      })
      if (response.statusCode < 200 || response.statusCode >= 300) {
        handlers.onError?.(new ApiError(response.data, response.statusCode))
      } else if (typeof response.data === 'string' && !parser.done()) {
        parser.push(response.data)
      }
    },
    fail(error) {
      logAPI(console, 'stream_failed', { method: 'POST', path, duration_ms: Date.now() - startedAt, error: error.errMsg || String(error) })
      if (!String(error.errMsg || '').includes('abort')) handlers.onError?.(error)
    },
  })
  task.onChunkReceived((event) => parser.push(decoder.push(event.data)))
  return task
}

module.exports = { ApiError, askStream, auth, buildURL, createAPIClient, createAuthAPI, logAPI, normalizeBaseUrl, pollIndexJob, ...client }
