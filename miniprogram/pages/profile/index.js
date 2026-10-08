const api = require('../../services/api')
const { parseWebLoginPayload } = require('../../services/auth')
const { formatBytes } = require('../../utils/format')

const app = getApp()

function percent(value, total) {
  if (!total) return '0%'
  return `${Math.min(100, Math.round((Number(value) / Number(total)) * 100))}%`
}

Page({
  data: {
    online: false, loading: true, error: '', tab: 'usage',
    user: {}, avatarLetter: '知', config: {}, usage: {},
    storageLabel: '0 B / 0 B', storageProgress: '0%',
    tokenLabel: '0 / 0', tokenProgress: '0%',
    ownerLibraries: [], libraryIndex: 0,
    settings: { chunk_size: 800, chunk_overlap: 100, top_k: 5, similarity_threshold: 0.3 },
    thresholdLabel: '0.30', reranker: true, saving: false,
    sessions: [], audits: [],
  },

  onShow() { this.refresh() },

  async refresh() {
    this.setData({ loading: true, error: '' })
    try {
      await app.ensureReady()
      const [user, usage, libraries, sessions, auditResult, config] = await Promise.all([
        api.me(), api.usageSummary(), api.libraries({ limit: 100 }),
        api.request('/api/v1/me/sessions'), api.auditLogs(), api.config().catch(() => ({})),
      ])
      const ownerLibraries = libraries.owned || []
      this.setData({
        online: true, loading: false, user, usage, ownerLibraries,
        avatarLetter: (user.nickname || '知').slice(0, 1),
        config, sessions: sessions.sessions || [], audits: auditResult.audit_logs || [],
        storageLabel: `${formatBytes(usage.storage_bytes)} / ${formatBytes(usage.storage_quota_bytes)}`,
        storageProgress: percent(usage.storage_bytes, usage.storage_quota_bytes),
        tokenLabel: `${Number(usage.total_tokens || 0).toLocaleString('zh-CN')} / ${Number(usage.monthly_token_quota || 0).toLocaleString('zh-CN')}`,
        tokenProgress: percent(usage.total_tokens, usage.monthly_token_quota),
      })
      if (ownerLibraries.length) await this.loadSettings(ownerLibraries[this.data.libraryIndex]?.id || ownerLibraries[0].id)
    } catch (error) { this.setData({ online: false, loading: false, error: error.message }) }
  },

  switchTab(event) { this.setData({ tab: event.currentTarget.dataset.tab }) },

  async onLibraryChange(event) {
    const libraryIndex = Number(event.detail.value)
    this.setData({ libraryIndex })
    await this.loadSettings(this.data.ownerLibraries[libraryIndex].id)
  },

  async loadSettings(libraryID) {
    if (!libraryID) return
    try {
      const settings = await api.librarySettings(libraryID)
      this.setData({ settings, thresholdLabel: Number(settings.similarity_threshold).toFixed(2) })
    } catch (error) { wx.showToast({ title: error.message, icon: 'none' }) }
  },

  onTopK(event) { this.setData({ 'settings.top_k': Number(event.detail.value) }) },
  onThreshold(event) {
    const value = Number(event.detail.value) / 100
    this.setData({ 'settings.similarity_threshold': value, thresholdLabel: value.toFixed(2) })
  },
  onReranker(event) { this.setData({ reranker: event.detail.value }) },

  async saveSettings() {
    const library = this.data.ownerLibraries[this.data.libraryIndex]
    if (!library) return
    this.setData({ saving: true })
    try {
      const result = await api.updateLibrarySettings(library.id, this.data.settings)
      wx.showToast({ title: result.requires_reindex ? '已保存，建议重新索引' : '检索参数已保存', icon: 'none' })
    } catch (error) { wx.showToast({ title: error.message, icon: 'none' }) }
    finally { this.setData({ saving: false }) }
  },

  async exportBackup() {
    wx.showLoading({ title: '正在整理备份' })
    try {
      const documents = await api.documents({ limit: 100 })
      const backup = {
        exported_at: new Date().toISOString(),
        user: this.data.user,
        libraries: this.data.ownerLibraries,
        documents: documents.documents || [],
        usage: this.data.usage,
        audit_logs: this.data.audits,
      }
      await new Promise((resolve, reject) => wx.setClipboardData({ data: JSON.stringify(backup, null, 2), success: resolve, fail: reject }))
      wx.showToast({ title: '备份 JSON 已复制', icon: 'success' })
    } catch (error) { wx.showToast({ title: error.message, icon: 'none' }) }
    finally { wx.hideLoading() }
  },

  scanWebLogin() {
    wx.scanCode({
      scanType: ['qrCode'],
      success: async (result) => {
        try {
          const ticket = parseWebLoginPayload(result.result)
          await api.confirmWebTicket(ticket.id, ticket.secret)
          wx.showToast({ title: 'Web 登录已确认', icon: 'success' })
        } catch (error) { wx.showToast({ title: error.message, icon: 'none', duration: 3000 }) }
      },
    })
  },

  logout() {
    wx.showModal({
      title: '退出当前微信登录？',
      content: '本机刷新令牌会立即清除，稍后可重新授权登录。',
      confirmColor: '#E04F62',
      success: async (result) => {
        if (!result.confirm) return
        await app.logout()
        wx.showToast({ title: '已安全退出', icon: 'success' })
      },
    })
  },
})
