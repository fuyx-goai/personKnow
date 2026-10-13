const api = require('./services/api')

const DEFAULT_LOCAL_API_URL = 'http://127.0.0.1:8080'
const TEMPORARY_LOCAL_API_URL = 'http://127.0.0.1:18080'

function resolveAPIBaseUrl(storedValue) {
  const normalized = String(storedValue || '').trim().replace(/\/+$/, '')
  // 18080 是本地调试时的临时端口；项目恢复到 8080 后自动迁移旧缓存，
  // 否则 wx 存储中的地址会持续覆盖代码默认值，导致小程序一直请求旧网关。
  return normalized === TEMPORARY_LOCAL_API_URL ? DEFAULT_LOCAL_API_URL : normalized || DEFAULT_LOCAL_API_URL
}

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
    const storedApiBaseUrl = wx.getStorageSync('apiBaseUrl')
    this.globalData.apiBaseUrl = resolveAPIBaseUrl(storedApiBaseUrl)
    if (this.globalData.apiBaseUrl !== storedApiBaseUrl) wx.setStorageSync('apiBaseUrl', this.globalData.apiBaseUrl)
    // 启动时只恢复已授权会话，首次 wx.login 必须等用户点击授权按钮。
    this.bootstrapPromise = api.auth.hasSession() ? this.restoreSession() : null
  },

  async restoreSession() {
    try {
      // 先检查完整 v1 网关，再使用本地 refresh token 恢复会话。
      await api.requireMiniProgram()
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
    // 未授权时统一回到登录页，业务页不得隐式触发 wx.login。
    if (this.globalData.authReady) return this.globalData
    if (!api.auth.hasSession()) {
      wx.reLaunch({ url: '/pages/login/index' })
      throw new Error('AUTH_REQUIRED')
    }
    if (!this.bootstrapPromise) this.bootstrapPromise = this.restoreSession()
    return this.bootstrapPromise
  },

  async authorize(profile = {}) {
    // 用户主动点击授权后才请求微信 code，并交由 Go 网关换取自有会话。
    await api.requireMiniProgram()
    const account = await api.auth.login({ ...profile, device_label: '微信小程序' })
    this.globalData.user = account.user || api.auth.user()
    this.globalData.authReady = true
    this.bootstrapPromise = Promise.resolve(this.globalData)
    await this.refreshCore()
    return this.globalData
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
