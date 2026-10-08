const ITEMS = [
  { key: 'chat', label: '问答对答', icon: '↗', url: '/pages/chat/index' },
  { key: 'libraries', label: '知识库体系', icon: '⌘', url: '/pages/libraries/index' },
  { key: 'sources', label: '原资料操作', icon: '▤', url: '/pages/sources/index' },
  { key: 'profile', label: '我的空间', icon: '●', url: '/pages/profile/index' },
]

Component({
  properties: { current: { type: String, value: 'chat' } },
  data: { items: ITEMS },
  methods: {
    navigate(event) {
      const item = this.data.items.find((entry) => entry.key === event.currentTarget.dataset.key)
      if (!item || item.key === this.properties.current) return
      wx.redirectTo({ url: item.url })
    },
  },
})
