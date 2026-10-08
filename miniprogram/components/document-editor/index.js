Component({
  properties: {
    open: { type: Boolean, value: false },
    saving: { type: Boolean, value: false },
    document: {
      type: Object,
      value: null,
      observer(value) {
        this.setData({
          displayName: value?.display_name || value?.original_name || '',
          tags: (value?.tags || []).join('，'),
        })
      },
    },
    content: { type: String, value: '', observer(value) { this.setData({ text: value || '' }) } },
  },
  data: { displayName: '', tags: '', text: '' },
  methods: {
    onName(event) { this.setData({ displayName: event.detail.value }) },
    onTags(event) { this.setData({ tags: event.detail.value }) },
    onText(event) { this.setData({ text: event.detail.value }) },
    close() { if (!this.data.saving) this.triggerEvent('close') },
    stopTap() {},
    submit() {
      if (!this.data.displayName.trim() || !this.data.text.trim() || this.data.saving) return
      const tags = this.data.tags.split(/[，,]/).map((item) => item.trim()).filter(Boolean)
      this.triggerEvent('submit', { displayName: this.data.displayName.trim(), tags, text: this.data.text })
    },
  },
})
