Component({
  properties: {
    title: { type: String, value: '智深专属知识库' },
    subtitle: { type: String, value: '' },
    online: { type: Boolean, value: false },
  },
  data: {
    statusBarHeight: 20,
    navHeight: 44,
    rightInset: 96,
  },
  lifetimes: {
    attached() {
      const info = wx.getWindowInfo ? wx.getWindowInfo() : wx.getSystemInfoSync()
      const capsule = wx.getMenuButtonBoundingClientRect?.()
      const statusBarHeight = info.statusBarHeight || 20
      const navHeight = capsule ? (capsule.top - statusBarHeight) * 2 + capsule.height : 44
      const rightInset = capsule ? info.windowWidth - capsule.left + 12 : 96
      this.setData({ statusBarHeight, navHeight, rightInset })
    },
  },
})
