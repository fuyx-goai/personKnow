<script setup>
// ConfirmDialog —— 二次确认弹层
// 删除是不可逆的（尤其"清空整库"），所以不用 window.confirm：
// 那个框又丑又会被浏览器拦，自己做一个更好控制措辞和视觉。
import { onBeforeUnmount, watch } from 'vue'

const props = defineProps({
  open: { type: Boolean, default: false },
  title: { type: String, default: '确认操作' },
  body: { type: String, default: '' },
  confirmText: { type: String, default: '确定' },
})

const emit = defineEmits(['update:open', 'confirm'])

function close() {
  emit('update:open', false)
}

function confirm() {
  emit('confirm')
  close()
}

function onKeydown(e) {
  if (e.key === 'Escape') close()
}

// 只在打开时挂键盘监听，关掉就摘掉，避免常驻
watch(
  () => props.open,
  (open) => {
    if (open) window.addEventListener('keydown', onKeydown)
    else window.removeEventListener('keydown', onKeydown)
  },
)

onBeforeUnmount(() => window.removeEventListener('keydown', onKeydown))
</script>

<template>
  <Transition name="mask">
    <div v-if="open" class="mask" @click.self="close">
      <div class="dialog" role="dialog" aria-modal="true">
        <h3 class="dialog__title">{{ title }}</h3>
        <p class="dialog__body">{{ body }}</p>
        <div class="dialog__actions">
          <button class="btn btn--ghost" type="button" @click="close">取消</button>
          <button class="btn btn--danger" type="button" @click="confirm">{{ confirmText }}</button>
        </div>
      </div>
    </div>
  </Transition>
</template>

<style scoped>
.mask {
  position: fixed;
  inset: 0;
  z-index: 40;
  display: grid;
  place-items: center;
  padding: 1.5rem;
  background: rgba(18, 22, 28, 0.38);
  backdrop-filter: blur(5px);
}

.dialog {
  width: min(26rem, 100%);
  padding: 1.5rem 1.6rem 1.3rem;
  border: 1px solid var(--line);
  border-radius: 18px;
  background: var(--surface);
  box-shadow: var(--shadow-lg);
}

.dialog__title {
  margin: 0 0 0.7rem;
  font-size: 1.1rem;
  font-weight: 650;
}

.dialog__body {
  margin: 0 0 1.4rem;
  font-size: 0.86rem;
  line-height: 1.8;
  color: var(--text-2);
}

.dialog__actions {
  display: flex;
  justify-content: flex-end;
  gap: 0.6rem;
}

.mask-enter-active,
.mask-leave-active {
  transition: opacity 0.2s var(--ease);
}

.mask-enter-from,
.mask-leave-to {
  opacity: 0;
}

.mask-enter-active .dialog,
.mask-leave-active .dialog {
  transition: transform 0.24s var(--ease);
}

.mask-enter-from .dialog {
  transform: translateY(14px) scale(0.98);
}
</style>
