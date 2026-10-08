const { ApiError } = require('./api')

function wxUploadFile(options) {
  return new Promise((resolve, reject) => {
    wx.uploadFile({ ...options, success: resolve, fail: reject })
  })
}

function parsePayload(data) {
  if (typeof data !== 'string') return data || {}
  try { return JSON.parse(data) } catch (_) { return { message: data } }
}

function createUploader(options = {}) {
  const uploadFile = options.uploadFile || wxUploadFile
  const baseUrl = options.baseUrl || (() => getApp().globalData.apiBaseUrl)
  const auth = options.auth

  async function uploadDocument(filePath, libraryID, filename, retried = false) {
    const response = await uploadFile({
      url: `${baseUrl()}/api/v1/documents`,
      filePath,
      name: 'file',
      formData: { library_id: libraryID },
      header: { Authorization: `Bearer ${auth?.accessToken?.() || ''}` },
      timeout: 120000,
      ...(filename ? { filename } : {}),
    })
    const payload = parsePayload(response.data)
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
