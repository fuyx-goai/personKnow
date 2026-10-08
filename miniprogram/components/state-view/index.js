Component({
  properties: {
    type: { type: String, value: 'empty' },
    title: { type: String, value: '暂无内容' },
    message: { type: String, value: '' },
    actionText: { type: String, value: '' },
  },
  methods: { action() { this.triggerEvent('action') } },
})
