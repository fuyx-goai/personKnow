const api = require('../../services/api')
const { formatBytes } = require('../../utils/format')

const app = getApp()
const ACCENTS = ['green', 'purple', 'orange', 'blue']

function displayLibrary(library, index, readOnly) {
  return {
    ...library,
    accent: ACCENTS[index % ACCENTS.length],
    readOnly,
    ownerLabel: library.owner_nickname || '公开贡献者',
    sizeLabel: formatBytes(library.storage_bytes),
    tokenLabel: Number(library.indexed_tokens || 0).toLocaleString('zh-CN'),
  }
}

Page({
  data: {
    online: false,
    loading: true,
    error: '',
    query: '',
    categoryIndex: 0,
    categoryOptions: ['全部分类'],
    libraries: [],
    visibleLibraries: [],
    totals: { libraries: 0, documents: 0, chunks: 0 },
    formOpen: false,
    saving: false,
  },

  onShow() { this.refresh() },

  async refresh() {
    this.setData({ loading: true, error: '' })
    try {
      // 知识库接口受账号鉴权保护，统一等待 App 完成登录和核心状态恢复。
      await app.ensureReady()
      const result = await api.libraries({ limit: 100 })
      const owned = (result.owned || []).map((item, index) => displayLibrary(item, index, false))
      const publicItems = (result.public || []).map((item, index) => displayLibrary(item, owned.length + index, true))
      const libraries = owned.concat(publicItems)
      const categories = [...new Set(libraries.map((item) => item.category).filter(Boolean))]
      const totals = libraries.reduce((sum, item) => ({
        libraries: sum.libraries + 1,
        documents: sum.documents + Number(item.document_count || 0),
        chunks: sum.chunks + Number(item.chunk_count || 0),
      }), { libraries: 0, documents: 0, chunks: 0 })
      this.setData({ online: true, loading: false, libraries, totals, categoryOptions: ['全部分类'].concat(categories) }, () => this.applyFilters())
    } catch (error) {
      this.setData({ online: false, loading: false, error: error.message })
    }
  },

  onSearch(event) { this.setData({ query: event.detail.value }, () => this.applyFilters()) },
  onCategory(event) { this.setData({ categoryIndex: Number(event.detail.value) }, () => this.applyFilters()) },

  applyFilters() {
    const query = this.data.query.trim().toLowerCase()
    const category = this.data.categoryOptions[this.data.categoryIndex]
    const visibleLibraries = this.data.libraries.filter((item) => {
      if (category !== '全部分类' && item.category !== category) return false
      return !query || `${item.name} ${item.description}`.toLowerCase().includes(query)
    })
    this.setData({ visibleLibraries })
  },

  openForm() { this.setData({ formOpen: true }) },
  closeForm() { this.setData({ formOpen: false }) },

  async createLibrary(event) {
    this.setData({ saving: true })
    try {
      await api.createLibrary(event.detail)
      this.setData({ formOpen: false })
      wx.showToast({ title: '知识库已创建', icon: 'success' })
      await this.refresh()
    } catch (error) {
      wx.showToast({ title: error.message, icon: 'none', duration: 3000 })
    } finally {
      this.setData({ saving: false })
    }
  },

  openChat(event) {
    wx.redirectTo({ url: `/pages/chat/index?library=${event.currentTarget.dataset.id}` })
  },

  manage(event) {
    wx.redirectTo({ url: `/pages/sources/index?library=${event.currentTarget.dataset.id}` })
  },

  remove(event) {
    const { id, name } = event.currentTarget.dataset
    wx.showModal({
      title: '删除这个知识库？',
      content: `「${name}」及其中资料会进入异步清理流程，此操作不可撤销。`,
      confirmColor: '#E04F62',
      success: async (result) => {
        if (!result.confirm) return
        try {
          await api.deleteLibrary(id)
          wx.showToast({ title: '已提交删除', icon: 'success' })
          await this.refresh()
        } catch (error) { wx.showToast({ title: error.message, icon: 'none' }) }
      },
    })
  },
})
