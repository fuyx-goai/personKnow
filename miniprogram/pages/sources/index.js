const api = require('../../services/api')
const { groupChunksBySource } = require('../../utils/library')
const { formatBytes, shortName } = require('../../utils/format')

const app = getApp()

Page({
  data: {
    online: false,
    loading: true,
    error: '',
    query: '',
    sourceIndex: 0,
    sourceOptions: ['全部知识库'],
    groups: [],
    visibleGroups: [],
    ingestOpen: false,
    ingestPath: './docs',
    ingesting: false,
  },

  onLoad(options) { this.initialSource = options.source ? decodeURIComponent(options.source) : '' },
  onShow() { this.refresh() },

  async refresh() {
    this.setData({ loading: true, error: '' })
    try {
      await app.refreshCore()
      const groups = groupChunksBySource(app.globalData.chunks).map((group) => ({
        ...group,
        displayName: shortName(group.source, 24),
        fileType: group.source.toLowerCase().endsWith('.pdf') ? 'PDF' : 'DOC',
        sizeLabel: formatBytes(group.bytes),
        preview: group.items[0]?.content?.slice(0, 96) || '已建立语义索引',
      }))
      const sourceOptions = ['全部知识库'].concat(groups.map((item) => item.source))
      const sourceIndex = this.initialSource ? Math.max(0, sourceOptions.indexOf(this.initialSource)) : this.data.sourceIndex
      this.setData({ online: true, loading: false, groups, sourceOptions, sourceIndex }, () => this.applyFilters())
      this.initialSource = ''
    } catch (error) {
      this.setData({ online: false, loading: false, error: error.message })
    }
  },

  onSearch(event) { this.setData({ query: event.detail.value }, () => this.applyFilters()) },
  onSourceChange(event) { this.setData({ sourceIndex: Number(event.detail.value) }, () => this.applyFilters()) },

  applyFilters() {
    const query = this.data.query.trim().toLowerCase()
    const source = this.data.sourceOptions[this.data.sourceIndex]
    const visibleGroups = this.data.groups.filter((group) => {
      if (source !== '全部知识库' && group.source !== source) return false
      return !query || group.source.toLowerCase().includes(query) || group.preview.toLowerCase().includes(query)
    })
    this.setData({ visibleGroups })
  },

  openIngest() { this.setData({ ingestOpen: true }) },
  closeIngest() { if (!this.data.ingesting) this.setData({ ingestOpen: false }) },
  stopTap() {},
  onPathInput(event) { this.setData({ ingestPath: event.detail.value }) },

  async ingest() {
    const path = this.data.ingestPath.trim()
    if (!path || this.data.ingesting) return
    this.setData({ ingesting: true })
    try {
      const result = await api.ingest(path)
      wx.showToast({ title: `已入库 ${result.totalChunks || 0} 段`, icon: 'success' })
      this.setData({ ingestOpen: false })
      await this.refresh()
    } catch (error) {
      wx.showToast({ title: error.message, icon: 'none', duration: 3000 })
    } finally {
      this.setData({ ingesting: false })
    }
  },

  remove(event) {
    const source = event.currentTarget.dataset.source
    wx.showModal({
      title: '删除这份资料？',
      content: `将删除「${source}」的全部知识片段，原文件不会被删除。`,
      confirmColor: '#E04F62',
      success: async (result) => {
        if (!result.confirm) return
        try {
          await api.removeSource(source)
          wx.showToast({ title: '已删除', icon: 'success' })
          await this.refresh()
        } catch (error) {
          wx.showToast({ title: error.message, icon: 'none' })
        }
      },
    })
  },

  unavailable() { wx.showToast({ title: '需要后端文件管理接口', icon: 'none' }) },
})
