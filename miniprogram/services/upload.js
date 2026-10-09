const { ApiError, buildURL, logAPI } = require('./api')

function wxUploadFile(options) {
  return new Promise((resolve, reject) => {
    wx.uploadFile({ ...options, success: resolve, fail: reject })
  })
}

function parsePayload(data) {
  // wx.uploadFile 的响应通常是字符串，与 wx.request 的对象响应不同，需要在边界统一解析。
  if (typeof data !== 'string') return data || {}
  try { return JSON.parse(data) } catch (_) { return { message: data } }
}

function createUploader(options = {}) {
  const uploadFile = options.uploadFile || wxUploadFile
  const baseUrl = options.baseUrl || (() => getApp().globalData.apiBaseUrl)
  const auth = options.auth
  const logger = options.logger || console

  async function uploadDocument(filePath, libraryID, filename, retried = false) {
    const path = '/api/v1/documents'
    const startedAt = Date.now()
    logAPI(logger, 'upload_started', { method: 'POST', path, retried })
    let response
    try {
      response = await uploadFile({
        url: buildURL(baseUrl(), path), filePath, name: 'file',
        formData: { library_id: libraryID },
        header: { Authorization: `Bearer ${auth?.accessToken?.() || ''}` },
        timeout: 120000, ...(filename ? { filename } : {}),
      })
    } catch (error) {
      logAPI(logger, 'upload_failed', { method: 'POST', path, duration_ms: Date.now() - startedAt, error: error.errMsg || error.message || String(error) })
      throw error
    }
    logAPI(logger, 'upload_finished', { method: 'POST', path, status_code: response.statusCode, duration_ms: Date.now() - startedAt })
    const payload = parsePayload(response.data)
    // refresh token 会在服务端轮换，因此上传鉴权失败时最多刷新并重放一次。
    if (response.statusCode === 401 && auth?.refresh && !retried) {
      await auth.refresh()
      return uploadDocument(filePath, libraryID, filename, true)
    }
    if (response.statusCode < 200 || response.statusCode >= 300) {
      throw new ApiError(payload, response.statusCode)
    }
    return payload
  }

  return { uploadDocument }
}

const uploader = createUploader({
  auth: require('./api').auth,
  baseUrl: () => getApp().globalData.apiBaseUrl || 'http://127.0.0.1:8080',
})

module.exports = { createUploader, ...uploader }
