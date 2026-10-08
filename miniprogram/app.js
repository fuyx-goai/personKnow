const api = require('./services/api')

App({
  globalData: {
    apiBaseUrl: '',
    online: false,
    config: {},
    stats: { chunks: 0 },
    chunks: [],
    lastError: '',
  },

  onLaunch() {
    this.globalData.apiBaseUrl = wx.getStorageSync('apiBaseUrl') || 'http://127.0.0.1:8080'
    this.refreshCore().catch(() => {})
  },

  async refreshCore() {
    try {
      const [config, stats, chunkResult] = await Promise.all([
        api.config(),
        api.stats(),
        api.chunks(1000),
      ])
      this.globalData.online = true
      this.globalData.config = config || {}
      this.globalData.stats = stats || { chunks: 0 }
      this.globalData.chunks = chunkResult?.chunks || []
      this.globalData.lastError = ''
      return this.globalData
    } catch (error) {
      this.globalData.online = false
      this.globalData.lastError = error.message
      throw error
    }
  },

  setApiBaseUrl(value) {
    const url = String(value || '').trim().replace(/\/+$/, '')
    this.globalData.apiBaseUrl = url
    wx.setStorageSync('apiBaseUrl', url)
  },
})
