<script setup>
// ToastHost —— 右下角的轻量提示
// 挂在根组件上一次即可，任何地方调 notify() 都能弹出来。
import { state } from '../store'
</script>

<template>
  <Transition name="toast">
    <div
      v-if="state.toast"
      :key="state.toast.id"
      class="toast"
      :class="`is-${state.toast.kind}`"
      role="status"
      aria-live="polite"
    >
      <span class="toast__mark">{{ state.toast.kind === 'bad' ? '!' : '✓' }}</span>
      <span class="toast__text">{{ state.toast.text }}</span>
    </div>
  </Transition>
</template>

<style scoped>
.toast {
  position: fixed;
  right: 1.6rem;
  bottom: 1.6rem;
  z-index: 30;
  display: flex;
  align-items: flex-start;
  gap: 0.65rem;
  max-width: min(24rem, calc(100vw - 3rem));
  padding: 0.8rem 1rem;
  border: 1px solid var(--line);
  border-radius: 14px;
  background: rgba(255, 255, 255, 0.96);
  box-shadow: var(--shadow-lg);
  font-size: 0.84rem;
  line-height: 1.6;
}

.toast.is-ok {
  border-color: rgba(33, 163, 102, 0.25);
}

.toast.is-bad {
  border-color: rgba(229, 72, 77, 0.3);
}

.toast__mark {
  flex: 0 0 auto;
  display: grid;
  place-items: center;
  width: 18px;
  height: 18px;
  margin-top: 0.1rem;
  border-radius: 50%;
  background: var(--success);
  color: #fff;
  font-size: 0.66rem;
  line-height: 1;
}

.toast.is-bad .toast__mark {
  background: var(--danger);
}

.toast__text {
  word-break: break-word;
}

.toast-enter-active,
.toast-leave-active {
  transition: opacity 0.24s var(--ease), transform 0.24s var(--ease);
}

.toast-enter-from,
.toast-leave-to {
  opacity: 0;
  transform: translateY(12px);
}
</style>
