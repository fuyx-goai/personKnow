Component({
  properties: {
    open: { type: Boolean, value: false },
    saving: { type: Boolean, value: false },
  },
  data: {
    name: '',
    category: '专业知识',
    description: '',
    visibility: 'private',
  },
  methods: {
    onName(event) { this.setData({ name: event.detail.value }) },
    onCategory(event) { this.setData({ category: event.detail.value }) },
    onDescription(event) { this.setData({ description: event.detail.value }) },
    setVisibility(event) { this.setData({ visibility: event.currentTarget.dataset.value }) },
    close() { if (!this.data.saving) this.triggerEvent('close') },
    stopTap() {},
    submit() {
      if (!this.data.name.trim() || this.data.saving) return
      this.triggerEvent('submit', {
        name: this.data.name.trim(),
        category: this.data.category.trim(),
        description: this.data.description.trim(),
        visibility: this.data.visibility,
      })
    },
  },
})
