const { groupChunksBySource } = require('../../utils/library')

const app = getApp()

Page({
  data: {
    online: false,
    loading: true,
    error: '',
    apiBaseUrl: '',
    sourceCount: 0,
    chunkCount: 0,
    progressWidth: '0%',
    config: {},
  },

  onShow() {
    this.setData({ apiBaseUrl: app.globalData.apiBaseUrl })
    this.refresh()
  },

  async refresh() {
    this.setData({ loading: true, error: '' })
    try {
      await app.refreshCore()
      this.setData({
        online: true,
        loading: false,
        config: app.globalData.config,
        chunkCount: app.globalData.stats.chunks || 0,
        progressWidth: app.globalData.stats.chunks ? '64%' : '0%',
        sourceCount: groupChunksBySource(app.globalData.chunks).length,
      })
    } catch (error) {
      this.setData({ online: false, loading: false, error: error.message, config: app.globalData.config || {} })
    }
  },

  onUrlInput(event) { this.setData({ apiBaseUrl: event.detail.value }) },

  saveUrl() {
    if (!this.data.apiBaseUrl.trim()) return
    app.setApiBaseUrl(this.data.apiBaseUrl)
    wx.showToast({ title: '接口地址已保存', icon: 'success' })
    this.refresh()
  },

  unavailable() { wx.showToast({ title: '需要后端账号能力', icon: 'none' }) },
})
