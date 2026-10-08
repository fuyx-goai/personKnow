const { createSSEParser, createUtf8Decoder } = require('../utils/sse')

function baseUrl() {
  return getApp().globalData.apiBaseUrl || 'http://127.0.0.1:8080'
}

function errorMessage(data, statusCode) {
  if (data && typeof data === 'object' && data.error) return data.error
  if (typeof data === 'string' && data) {
    try { return JSON.parse(data).error || data } catch { return data }
  }
  return `请求失败（HTTP ${statusCode || 0}）`
}

function request(path, options = {}) {
  return new Promise((resolve, reject) => {
    wx.request({
      url: `${baseUrl()}${path}`,
      method: options.method || 'GET',
      data: options.data,
      header: { 'Content-Type': 'application/json' },
      timeout: options.timeout || 30000,
      success(response) {
        if (response.statusCode >= 200 && response.statusCode < 300) resolve(response.data)
        else reject(new Error(errorMessage(response.data, response.statusCode)))
      },
      fail(error) { reject(new Error(error.errMsg || '无法连接知识库服务')) },
    })
  })
}

function askStream(question, handlers = {}) {
  const decoder = createUtf8Decoder()
  const parser = createSSEParser(handlers)
  let receivedChunk = false
  const task = wx.request({
    url: `${baseUrl()}/api/chat/stream`,
    method: 'POST',
    data: { question },
    header: { 'Content-Type': 'application/json' },
    enableChunked: true,
    timeout: 120000,
    success(response) {
      if (response.statusCode < 200 || response.statusCode >= 300) {
        handlers.onError?.(errorMessage(response.data, response.statusCode))
        return
      }
      if (!receivedChunk && typeof response.data === 'string') parser.push(response.data)
      if (!parser.done()) handlers.onDone?.()
    },
    fail(error) {
      if (!String(error.errMsg || '').includes('abort')) handlers.onError?.(error.errMsg || '问答请求失败')
    },
  })
  task.onChunkReceived((event) => {
    receivedChunk = true
    parser.push(decoder.push(event.data))
  })
  return task
}

module.exports = {
  health: () => request('/api/health'),
  config: () => request('/api/config'),
  stats: () => request('/api/stats'),
  chunks: (limit = 1000) => request(`/api/chunks?limit=${limit}`),
  ingest: (path) => request('/api/ingest', { method: 'POST', data: { path }, timeout: 120000 }),
  removeSource: (source) => request(`/api/chunks?source=${encodeURIComponent(source)}`, { method: 'DELETE' }),
  askStream,
}
