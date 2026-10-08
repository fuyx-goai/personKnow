const api = require('../../services/api')
const upload = require('../../services/upload')
const { formatBytes } = require('../../utils/format')

const app = getApp()
const STATUS_LABELS = {
  queued: '等待索引', processing: '正在索引', ready: '已切块就绪',
  failed: '索引失败', deleting: '正在删除', delete_failed: '删除失败',
}

function displayDocument(document, libraryMap) {
  const library = libraryMap.get(document.library_id) || {}
  return {
    ...document,
    libraryName: library.name || '未知知识库',
    readOnly: library.access !== 'owner',
    formatLabel: String(document.format || 'file').toUpperCase(),
    sizeLabel: formatBytes(document.original_bytes),
    statusLabel: STATUS_LABELS[document.status] || document.status,
    tokenLabel: Number(document.indexed_tokens || 0).toLocaleString('zh-CN'),
    updatedLabel: new Date(document.updated_at).toLocaleString('zh-CN', { hour12: false }),
  }
}

Page({
  data: {
    online: false, loading: true, error: '', query: '', libraryIndex: 0,
    libraryOptions: [{ id: '', name: '全部知识库', access: 'owner' }],
    documents: [], visibleDocuments: [],
    editorOpen: false, editorSaving: false, editingDocument: null, editingContent: '',
    workingID: '',
  },

  onLoad(options) { this.initialLibraryID = options.library || '' },
  onShow() { this.refresh() },

  async refresh() {
    this.setData({ loading: true, error: '' })
    try {
      await app.ensureReady()
      const [libraries, result] = await Promise.all([api.libraries({ limit: 100 }), api.documents({ limit: 100 })])
      const libraryOptions = [{ id: '', name: '全部知识库', access: 'owner' }]
        .concat(libraries.owned || [], libraries.public || [])
      const libraryMap = new Map(libraryOptions.map((item) => [item.id, item]))
      const documents = (result.documents || []).map((item) => displayDocument(item, libraryMap))
      let libraryIndex = this.data.libraryIndex
      if (this.initialLibraryID) libraryIndex = Math.max(0, libraryOptions.findIndex((item) => item.id === this.initialLibraryID))
      this.initialLibraryID = ''
      this.setData({ online: true, loading: false, libraryOptions, libraryIndex, documents }, () => this.applyFilters())
    } catch (error) {
      this.setData({ online: false, loading: false, error: error.message })
    }
  },

  onSearch(event) { this.setData({ query: event.detail.value }, () => this.applyFilters()) },
  onLibraryChange(event) { this.setData({ libraryIndex: Number(event.detail.value) }, () => this.applyFilters()) },

  applyFilters() {
    const query = this.data.query.trim().toLowerCase()
    const libraryID = this.data.libraryOptions[this.data.libraryIndex]?.id
    const visibleDocuments = this.data.documents.filter((item) => {
      if (libraryID && item.library_id !== libraryID) return false
      return !query || `${item.display_name} ${item.summary} ${(item.tags || []).join(' ')}`.toLowerCase().includes(query)
    })
    this.setData({ visibleDocuments })
  },

  chooseUpload() {
    const selected = this.data.libraryOptions[this.data.libraryIndex]
    const target = selected?.id && selected.access === 'owner'
      ? selected
      : this.data.libraryOptions.find((item) => item.id && item.access === 'owner')
    if (!target) {
      wx.showToast({ title: '请先创建私有知识库', icon: 'none' })
      return
    }
    wx.chooseMessageFile({
      count: 1,
      type: 'file',
      extension: ['pdf', 'docx', 'pptx', 'md', 'txt', 'html', 'csv'],
      success: (result) => this.uploadFile(result.tempFiles[0], target),
    })
  },

  async uploadFile(file, library) {
    wx.showLoading({ title: '正在上传' })
    try {
      const result = await upload.uploadDocument(file.path, library.id, file.name)
      wx.showToast({ title: '已进入索引队列', icon: 'success' })
      this.waitForJob(result.job_id)
      await this.refresh()
    } catch (error) { wx.showToast({ title: error.message, icon: 'none', duration: 3000 }) }
    finally { wx.hideLoading() }
  },

  async waitForJob(jobID) {
    if (!jobID) return
    try {
      const job = await api.pollIndexJob(() => api.indexJob(jobID))
      wx.showToast({ title: job.status === 'ready' ? '索引已完成' : '索引失败', icon: 'none' })
      await this.refresh()
    } catch (_) {}
  },

  async openEditor(event) {
    const item = this.data.documents.find((document) => document.id === event.currentTarget.dataset.id)
    if (!item || item.readOnly) return wx.showToast({ title: '公开资料仅支持查看', icon: 'none' })
    this.setData({ workingID: item.id })
    try {
      const result = await api.documentContent(item.id)
      this.setData({ editorOpen: true, editingDocument: item, editingContent: result.text || '' })
    } catch (error) { wx.showToast({ title: error.message, icon: 'none' }) }
    finally { this.setData({ workingID: '' }) }
  },

  closeEditor() { this.setData({ editorOpen: false, editingDocument: null, editingContent: '' }) },

  async saveEditor(event) {
    const document = this.data.editingDocument
    if (!document) return
    this.setData({ editorSaving: true })
    try {
      await api.updateDocument(document.id, { DisplayName: event.detail.displayName, Tags: event.detail.tags })
      const result = await api.editDocument(document.id, event.detail.text)
      this.closeEditor()
      wx.showToast({ title: '已保存，正在重建索引', icon: 'none' })
      this.waitForJob(result.job_id)
      await this.refresh()
    } catch (error) { wx.showToast({ title: error.message, icon: 'none', duration: 3000 }) }
    finally { this.setData({ editorSaving: false }) }
  },

  async reindex(event) {
    const id = event.currentTarget.dataset.id
    this.setData({ workingID: id })
    try {
      const result = await api.reindexDocument(id)
      wx.showToast({ title: '重新索引已开始', icon: 'none' })
      this.waitForJob(result.job_id)
    } catch (error) { wx.showToast({ title: error.message, icon: 'none' }) }
    finally { this.setData({ workingID: '' }) }
  },

  remove(event) {
    const { id, name } = event.currentTarget.dataset
    wx.showModal({
      title: '彻底删除此文件？',
      content: `将删除「${name}」及其全部向量，此操作不可撤销。`,
      confirmColor: '#E04F62',
      success: async (result) => {
        if (!result.confirm) return
        try {
          const job = await api.deleteDocument(id)
          this.waitForJob(job.job_id)
          await this.refresh()
        } catch (error) { wx.showToast({ title: error.message, icon: 'none' }) }
      },
    })
  },
})
