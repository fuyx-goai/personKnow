const { createAuthSession } = require('./auth')
const { createSSEParser, createUtf8Decoder } = require('../utils/sse')

class ApiError extends Error {
  constructor(payload = {}, statusCode = 0) {
    super(payload.message || payload.error || `请求失败（HTTP ${statusCode}）`)
    this.name = 'ApiError'
    this.code = payload.code || 'REQUEST_FAILED'
    this.requestId = payload.request_id || ''
    this.statusCode = statusCode
  }
}

function appBaseUrl() {
  return getApp().globalData.apiBaseUrl || 'http://127.0.0.1:8080'
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

function createAPIClient(options = {}) {
  const transport = options.transport || wxTransport
  const baseUrl = options.baseUrl || appBaseUrl
  const auth = options.auth

  async function request(path, requestOptions = {}, retried = false) {
    const token = auth?.accessToken?.()
    const response = await transport({
      url: `${baseUrl()}${path}`,
      method: requestOptions.method || 'GET',
      data: requestOptions.data,
      timeout: requestOptions.timeout || 30000,
      header: {
        'Content-Type': 'application/json',
        ...(token ? { Authorization: `Bearer ${token}` } : {}),
        ...(requestOptions.header || {}),
      },
    })
    if (response.statusCode === 401 && auth?.refresh && !retried && !requestOptions.skipRefresh) {
      await auth.refresh()
      return request(path, requestOptions, true)
    }
    if (response.statusCode < 200 || response.statusCode >= 300) {
      throw new ApiError(response.data, response.statusCode)
    }
    return response.data
  }

  return {
    auth,
    request,
    health: () => request('/api/health', { skipRefresh: true }),
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

function rawRequest(path, options = {}) {
  return wxTransport({
    url: `${appBaseUrl()}${path}`,
    method: options.method || 'POST',
    data: options.data,
    header: { 'Content-Type': 'application/json', ...(options.header || {}) },
    timeout: 30000,
  }).then((response) => {
    if (response.statusCode < 200 || response.statusCode >= 300) {
      throw new ApiError(response.data, response.statusCode)
    }
    return response.data
  })
}

const auth = createAuthSession({
  login: wxLogin,
  exchange: (data) => rawRequest('/api/v1/auth/wechat/login', { data }),
  refresh: (refreshToken) => rawRequest('/api/v1/auth/refresh', { data: { refresh_token: refreshToken } }),
  revoke: (token) => rawRequest('/api/v1/auth/logout', { header: { Authorization: `Bearer ${token}` } }),
})
const client = createAPIClient({ auth })

function askStream(sessionID, question, handlers = {}) {
  const decoder = createUtf8Decoder()
  const parser = createSSEParser(handlers)
  const task = wx.request({
    url: `${appBaseUrl()}/api/v1/chat/sessions/${sessionID}/messages/stream`,
    method: 'POST',
    data: { question },
    header: { 'Content-Type': 'application/json', Authorization: `Bearer ${auth.accessToken()}` },
    enableChunked: true,
    timeout: 120000,
    success(response) {
      if (response.statusCode < 200 || response.statusCode >= 300) {
        handlers.onError?.(new ApiError(response.data, response.statusCode))
      } else if (typeof response.data === 'string' && !parser.done()) {
        parser.push(response.data)
      }
    },
    fail(error) {
      if (!String(error.errMsg || '').includes('abort')) handlers.onError?.(error)
    },
  })
  task.onChunkReceived((event) => parser.push(decoder.push(event.data)))
  return task
}

module.exports = { ApiError, askStream, auth, createAPIClient, pollIndexJob, ...client }
