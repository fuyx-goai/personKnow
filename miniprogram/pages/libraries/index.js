const { groupChunksBySource } = require('../../utils/library')
const { formatBytes, shortName } = require('../../utils/format')

const app = getApp()
const ACCENTS = ['green', 'purple', 'orange', 'blue']

Page({
  data: { online: false, loading: true, error: '', query: '', total: 0, groups: [], visibleGroups: [] },

  onShow() { this.refresh() },

  async refresh() {
    this.setData({ loading: true, error: '' })
    try {
      await app.refreshCore()
      const groups = groupChunksBySource(app.globalData.chunks).map((group, index) => ({
        ...group,
        name: shortName(group.source, 22),
        sizeLabel: formatBytes(group.bytes),
        accent: ACCENTS[index % ACCENTS.length],
        description: group.items[0]?.content?.slice(0, 82) || '已建立语义索引，可直接进入知识问答。',
      }))
      this.setData({ online: true, loading: false, total: app.globalData.stats.chunks || 0, groups, visibleGroups: groups })
    } catch (error) {
      this.setData({ online: false, loading: false, error: error.message })
    }
  },

  onSearch(event) {
    const query = event.detail.value.trim().toLowerCase()
    const visibleGroups = this.data.groups.filter((item) => item.source.toLowerCase().includes(query))
    this.setData({ query, visibleGroups })
  },

  openChat() { wx.redirectTo({ url: '/pages/chat/index' }) },

  manage(event) {
    const source = encodeURIComponent(event.currentTarget.dataset.source)
    wx.redirectTo({ url: `/pages/sources/index?source=${source}` })
  },

  unavailable() { wx.showToast({ title: '多知识库能力需要后端支持', icon: 'none' }) },
})
