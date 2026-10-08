<script setup>
defineProps({
  name: { type: String, required: true },
  size: { type: [Number, String], default: 20 },
  strokeWidth: { type: [Number, String], default: 1.8 },
})
</script>

<template>
  <svg class="app-icon" :width="size" :height="size" viewBox="0 0 24 24" fill="none"
    stroke="currentColor" :stroke-width="strokeWidth" stroke-linecap="round"
    stroke-linejoin="round" aria-hidden="true">
    <template v-if="name === 'home'">
      <path d="m3 11 9-7 9 7" /><path d="M5.5 9.5V20h13V9.5" /><path d="M9.5 20v-6h5v6" />
    </template>
    <template v-else-if="name === 'upload'">
      <path d="M12 16V4" /><path d="m7.5 8.5 4.5-4.5 4.5 4.5" /><path d="M5 14v5h14v-5" />
    </template>
    <template v-else-if="name === 'chat'">
      <path d="M20 15a4 4 0 0 1-4 4H8l-5 3V8a4 4 0 0 1 4-4h9a4 4 0 0 1 4 4Z" />
    </template>
    <template v-else-if="name === 'library'">
      <path d="M4 5.5A2.5 2.5 0 0 1 6.5 3H10v17H6.5A2.5 2.5 0 0 1 4 17.5Z" /><path d="M20 5.5A2.5 2.5 0 0 0 17.5 3H14v17h3.5a2.5 2.5 0 0 0 2.5-2.5Z" />
    </template>
    <template v-else-if="name === 'panel'">
      <rect x="3" y="4" width="18" height="16" rx="3" /><path d="M9 4v16" />
    </template>
    <template v-else-if="name === 'search'">
      <circle cx="10.5" cy="10.5" r="6.5" /><path d="m16 16 4 4" />
    </template>
    <template v-else-if="name === 'send'">
      <path d="m4 4 17 8-17 8 3-8Z" /><path d="M7 12h14" />
    </template>
    <template v-else-if="name === 'plus'">
      <path d="M12 5v14M5 12h14" />
    </template>
    <template v-else-if="name === 'refresh'">
      <path d="M20 7v5h-5" /><path d="M18.5 16A8 8 0 1 1 19 8l1 4" />
    </template>
    <template v-else-if="name === 'trash'">
      <path d="M4 7h16M9 3h6l1 4H8l1-4Z" /><path d="m6 7 1 14h10l1-14M10 11v6M14 11v6" />
    </template>
    <template v-else-if="name === 'file'">
      <path d="M6 3h8l4 4v14H6Z" /><path d="M14 3v5h5M9 13h6M9 17h4" />
    </template>
    <template v-else-if="name === 'database'">
      <ellipse cx="12" cy="5.5" rx="8" ry="3.5" /><path d="M4 5.5v6c0 2 3.6 3.5 8 3.5s8-1.5 8-3.5v-6" /><path d="M4 11.5v6c0 2 3.6 3.5 8 3.5s8-1.5 8-3.5v-6" />
    </template>
    <template v-else-if="name === 'sparkle'">
      <path d="M12 2c.6 4 2 5.4 6 6-4 .6-5.4 2-6 6-.6-4-2-5.4-6-6 4-.6 5.4-2 6-6Z" /><path d="M19 15c.3 2 1 2.7 3 3-2 .3-2.7 1-3 3-.3-2-1-2.7-3-3 2-.3 2.7-1 3-3Z" />
    </template>
    <template v-else-if="name === 'stop'">
      <rect x="6" y="6" width="12" height="12" rx="2" />
    </template>
    <template v-else-if="name === 'more'">
      <circle cx="5" cy="12" r="1" fill="currentColor" stroke="none" /><circle cx="12" cy="12" r="1" fill="currentColor" stroke="none" /><circle cx="19" cy="12" r="1" fill="currentColor" stroke="none" />
    </template>
    <template v-else>
      <circle cx="12" cy="12" r="8" /><path d="M12 8v4l2.5 2" />
    </template>
  </svg>
</template>

<style scoped>
.app-icon { display: block; flex: 0 0 auto; }
</style>
