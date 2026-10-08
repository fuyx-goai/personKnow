const api = require('./services/api')

App({
  globalData: {
    apiBaseUrl: '',
    authReady: false,
    online: false,
    user: null,
    ownedLibraries: [],
    publicLibraries: [],
    usage: null,
    config: {},
    lastError: '',
  },

  onLaunch() {
    this.globalData.apiBaseUrl = wx.getStorageSync('apiBaseUrl') || 'http://127.0.0.1:8080'
    this.bootstrapPromise = this.bootstrap()
  },

  async bootstrap() {
    try {
      const account = await api.auth.restore({ device_label: '微信小程序' })
      this.globalData.user = account.user || api.auth.user()
      this.globalData.authReady = true
      await this.refreshCore()
      return this.globalData
    } catch (error) {
      this.bootstrapPromise = null
      this.globalData.authReady = false
      this.globalData.online = false
      this.globalData.lastError = error.message
      throw error
    }
  },

  async ensureReady() {
    if (this.globalData.authReady) return this.globalData
    if (!this.bootstrapPromise) this.bootstrapPromise = this.bootstrap()
    return this.bootstrapPromise
  },

  async refreshCore() {
    const [user, libraries, usage, config] = await Promise.all([
      api.me(),
      api.libraries({ limit: 100 }),
      api.usageSummary(),
      api.config().catch(() => ({})),
    ])
    Object.assign(this.globalData, {
      online: true,
      user,
      ownedLibraries: libraries.owned || [],
      publicLibraries: libraries.public || [],
      usage,
      config: config || {},
      lastError: '',
    })
    return this.globalData
  },

  async logout() {
    await api.auth.logout()
    this.bootstrapPromise = null
    Object.assign(this.globalData, {
      authReady: false,
      online: false,
      user: null,
      ownedLibraries: [],
      publicLibraries: [],
      usage: null,
    })
  },

  setApiBaseUrl(value) {
    const url = String(value || '').trim().replace(/\/+$/, '')
    this.globalData.apiBaseUrl = url
    wx.setStorageSync('apiBaseUrl', url)
  },
})
