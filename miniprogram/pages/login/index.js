const app = getApp()

Page({
  data: {
    restoring: true,
    authorizing: false,
    nickname: '',
    avatarLetter: '知',
    avatarUrl: '',
    error: '',
  },

  onLoad() { this.restoreSession() },

  async restoreSession() {
    if (app.globalData.authReady) return this.enterApp()
    try {
      if (!app.bootstrapPromise) throw new Error('AUTH_REQUIRED')
      await app.bootstrapPromise
      this.enterApp()
    } catch (error) {
      this.setData({
        restoring: false,
        error: error.message === 'AUTH_REQUIRED' ? '' : error.message,
      })
    }
  },

  onNickname(event) {
    const nickname = event.detail.value
    this.setData({ nickname, avatarLetter: nickname.trim().slice(0, 1) || '知' })
  },

  onChooseAvatar(event) {
    // chooseAvatar 返回的是本地临时路径，本期只用于登录页预览，不将路径传给后端持久化。
    this.setData({ avatarUrl: event.detail.avatarUrl || '' })
  },

  async authorize() {
    if (this.data.authorizing) return
    this.setData({ authorizing: true, error: '' })
    try {
      await app.authorize({ nickname: this.data.nickname.trim() })
      this.enterApp()
    } catch (error) {
      this.setData({ error: error.message || '微信授权登录失败，请重试' })
    } finally {
      this.setData({ authorizing: false, restoring: false })
    }
  },

  enterApp() { wx.reLaunch({ url: '/pages/chat/index' }) },
})
