const api = require('../../services/api')
const { formatScore } = require('../../utils/format')

const app = getApp()

function displayReference(reference) {
  return {
    ...reference,
    source: reference.source_name || reference.source || '原资料',
    snippet: reference.excerpt || reference.snippet || '',
    scoreLabel: formatScore(reference.similarity ?? reference.score),
  }
}

function displayMessage(message) {
  return {
    ...message,
    references: (message.references || []).map(displayReference),
    usageLabel: message.total_tokens ? `${Number(message.total_tokens).toLocaleString('zh-CN')} Tokens` : '',
  }
}

Page({
  data: {
    online: false, loading: true, question: '', busy: false, error: '', scrollTarget: '',
    config: {},
    libraryIndex: 0, libraries: [{ id: '', name: '全域专属知识库（混合检索）' }],
    sessions: [], currentSessionID: '', historyOpen: false,
    messages: [],
    suggestions: ['总结当前知识库的核心主题', '有哪些内容值得继续整理？', '解释资料中的关键技术术语'],
  },

  onLoad(options) { this.initialLibraryID = options.library || '' },
  onShow() { this.refreshData() },
  onUnload() { this.requestTask?.abort() },

  async refreshData() {
    this.setData({ loading: true, error: '' })
    try {
      await app.ensureReady()
      const [libraryResult, sessionResult] = await Promise.all([api.libraries({ limit: 100 }), api.chatSessions()])
      const libraries = [{ id: '', name: '全域专属知识库（混合检索）' }]
        .concat((libraryResult.owned || []).map((item) => ({ ...item, name: item.name })))
        .concat((libraryResult.public || []).map((item) => ({ ...item, name: `${item.name} · 公开` })))
      let libraryIndex = this.data.libraryIndex
      if (this.initialLibraryID) libraryIndex = Math.max(0, libraries.findIndex((item) => item.id === this.initialLibraryID))
      this.initialLibraryID = ''
      this.setData({ online: true, loading: false, config: app.globalData.config || {}, libraries, libraryIndex, sessions: sessionResult.sessions || [] })
    } catch (error) {
      this.setData({ online: false, loading: false, error: error.message })
    }
  },

  onInput(event) { this.setData({ question: event.detail.value }) },
  onLibraryChange(event) { this.setData({ libraryIndex: Number(event.detail.value) }); this.newChat() },
  useSuggestion(event) { this.setData({ question: event.currentTarget.dataset.value }, () => this.send()) },
  toggleHistory() { this.setData({ historyOpen: !this.data.historyOpen }) },
  closeHistory() { this.setData({ historyOpen: false }) },
  stopTap() {},

  async newChat() {
    this.requestTask?.abort()
    this.setData({ currentSessionID: '', messages: [], error: '', busy: false, historyOpen: false })
  },

  async ensureSession(question) {
    if (this.data.currentSessionID) return this.data.currentSessionID
    const library = this.data.libraries[this.data.libraryIndex]
    const command = library?.id
      ? { scope_type: 'single_library', library_id: library.id, title: question.slice(0, 36) }
      : { scope_type: 'global', title: question.slice(0, 36) }
    const session = await api.createChatSession(command)
    this.setData({ currentSessionID: session.id, sessions: [session].concat(this.data.sessions) })
    return session.id
  },

  async openSession(event) {
    const id = event.currentTarget.dataset.id
    this.setData({ loading: true, historyOpen: false })
    try {
      const result = await api.chatHistory(id)
      const messages = (result.messages || []).map(displayMessage)
      const libraryIndex = result.session.library_id
        ? Math.max(0, this.data.libraries.findIndex((item) => item.id === result.session.library_id)) : 0
      this.setData({ currentSessionID: id, messages, libraryIndex, loading: false, scrollTarget: `message-${messages.length - 1}` })
    } catch (error) { this.setData({ loading: false, error: error.message }) }
  },

  async send() {
    const question = this.data.question.trim()
    if (!question || this.data.busy) return
    try {
      const sessionID = await this.ensureSession(question)
      const id = Date.now()
      const messages = this.data.messages.concat([
        { id: `user-${id}`, role: 'user', content: question, references: [] },
        { id: `ai-${id}`, role: 'assistant', content: '', references: [], streaming: true, usageLabel: '' },
      ])
      const answerIndex = messages.length - 1
      this.setData({ messages, question: '', busy: true, error: '', scrollTarget: `message-${answerIndex}` })
      this.startStream(sessionID, question, answerIndex)
    } catch (error) { this.setData({ error: error.message, busy: false }) }
  },

  startStream(sessionID, question, answerIndex) {
    this.requestTask = api.askStream(sessionID, question, {
      onDelta: (delta) => this.appendDelta(answerIndex, delta),
      onReference: (reference) => this.appendReference(answerIndex, reference),
      onUsage: (usage) => this.setData({ [`messages[${answerIndex}].usageLabel`]: `${Number(usage.input_tokens + usage.output_tokens).toLocaleString('zh-CN')} Tokens` }),
      onError: (error) => this.finishAnswer(answerIndex, error.message || error.code || String(error)),
      onDone: () => this.finishAnswer(answerIndex),
    })
  },

  appendDelta(index, delta) {
    const current = this.data.messages[index]?.content || ''
    this.setData({ [`messages[${index}].content`]: `${current}${delta || ''}`, scrollTarget: `message-${index}` })
  },

  appendReference(index, reference) {
    const references = (this.data.messages[index]?.references || []).concat(displayReference(reference))
    this.setData({ [`messages[${index}].references`]: references })
  },

  finishAnswer(index, error = '') {
    this.setData({ busy: false, error, [`messages[${index}].streaming`]: false, scrollTarget: `message-${index}` })
    this.requestTask = null
  },

  stop() {
    this.requestTask?.abort()
    const index = this.data.messages.length - 1
    if (index >= 0) this.finishAnswer(index)
  },
})
