const api = require('../../services/api')
const { formatScore } = require('../../utils/format')

const app = getApp()

Page({
  data: {
    online: false,
    question: '',
    busy: false,
    error: '',
    scrollTarget: '',
    selectedLibrary: '全域专属知识库',
    libraryIndex: 0,
    libraries: ['全域专属知识库（混合检索）', '默认知识库'],
    messages: [{
      id: 'welcome',
      role: 'assistant',
      content: '你好，我会先检索你的知识库，再给出带来源依据的回答。',
      references: [],
    }],
    suggestions: ['总结当前知识库的核心主题', '有哪些内容值得继续整理？', '解释资料中的关键技术术语'],
  },

  onShow() {
    this.setData({ online: app.globalData.online })
    api.health().then(() => this.setData({ online: true })).catch(() => this.setData({ online: false }))
  },

  onUnload() { this.requestTask?.abort() },

  onInput(event) { this.setData({ question: event.detail.value }) },

  onLibraryChange(event) {
    const libraryIndex = Number(event.detail.value)
    this.setData({
      libraryIndex,
      selectedLibrary: libraryIndex === 0 ? '全域专属知识库' : '默认知识库',
    })
  },

  useSuggestion(event) {
    this.setData({ question: event.currentTarget.dataset.value }, () => this.send())
  },

  send() {
    const question = this.data.question.trim()
    if (!question || this.data.busy) return
    const id = Date.now()
    const messages = this.data.messages.concat([
      { id: `user-${id}`, role: 'user', content: question, references: [] },
      { id: `ai-${id}`, role: 'assistant', content: '', references: [], streaming: true },
    ])
    const answerIndex = messages.length - 1
    this.setData({ messages, question: '', busy: true, error: '', scrollTarget: `message-${answerIndex}` })

    this.requestTask = api.askStream(question, {
      onDelta: (delta) => {
        const path = `messages[${answerIndex}].content`
        this.setData({ [path]: `${this.data.messages[answerIndex].content}${delta}`, scrollTarget: `message-${answerIndex}` })
      },
      onReferences: (references) => {
        const values = (references || []).map((item) => ({ ...item, scoreLabel: formatScore(item.score) }))
        this.setData({ [`messages[${answerIndex}].references`]: values })
      },
      onError: (message) => this.finishAnswer(answerIndex, message),
      onDone: () => this.finishAnswer(answerIndex),
    })
  },

  finishAnswer(index, error = '') {
    this.setData({
      busy: false,
      error,
      [`messages[${index}].streaming`]: false,
      scrollTarget: `message-${index}`,
    })
    this.requestTask = null
  },

  stop() {
    this.requestTask?.abort()
    const index = this.data.messages.length - 1
    if (index >= 0) this.finishAnswer(index)
  },

  unavailable() {
    wx.showToast({ title: '需要后端多知识库支持', icon: 'none' })
  },
})
