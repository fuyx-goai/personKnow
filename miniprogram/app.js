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
    // 开发者工具可使用本机地址；真机必须改为局域网或 HTTPS 地址。
    this.globalData.apiBaseUrl = wx.getStorageSync('apiBaseUrl') || 'http://127.0.0.1:8080'
    this.bootstrapPromise = this.bootstrap()
  },

  async bootstrap() {
    try {
      // 先检查完整 v1 网关，再调用微信登录；旧二进制会在这里给出可操作的版本提示。
      await api.status()
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
    // 页面共用同一个启动 Promise，避免四个 tab 首次显示时重复登录和重复拉取核心数据。
    if (this.globalData.authReady) return this.globalData
    if (!this.bootstrapPromise) this.bootstrapPromise = this.bootstrap()
    return this.bootstrapPromise
  },

  async refreshCore() {
    // 核心数据并行请求，减少登录后首屏等待；单个可选配置失败不会阻断主数据。
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
    // 持久化时清理末尾斜杠，确保所有 API 都由统一 URL 拼接逻辑生成。
    const url = String(value || '').trim().replace(/\/+$/, '')
    this.globalData.apiBaseUrl = url
    wx.setStorageSync('apiBaseUrl', url)
  },
})
