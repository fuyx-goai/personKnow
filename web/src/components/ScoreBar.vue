<script setup>
// ScoreBar —— 相似度条形
// 让"这段资料到底有多相关"变成一个能一眼扫过的东西，而不是一串小数。
import { computed } from 'vue'
import { percent } from '../utils/format'

const props = defineProps({
  score: { type: Number, default: 0 },
})

// 留 4% 的最小宽度，分数很低时也看得见那根条
const width = computed(() => {
  const value = Number.isFinite(props.score) ? props.score : 0
  return `${Math.max(4, Math.min(100, Math.round(value * 100)))}%`
})
</script>

<template>
  <span class="score">
    <span class="score__track">
      <i class="score__fill" :style="{ width }" />
    </span>
    <span class="score__num is-mono">{{ percent(score) }}</span>
  </span>
</template>

<style scoped>
.score {
  display: inline-flex;
  align-items: center;
  gap: 0.5rem;
}

.score__track {
  position: relative;
  display: block;
  width: 3.6rem;
  height: 5px;
  border-radius: 99px;
  background: #e9edf3;
  overflow: hidden;
}

.score__fill {
  position: absolute;
  inset: 0 auto 0 0;
  border-radius: 99px;
  background: linear-gradient(90deg, #76a0ff, var(--primary));
  transform-origin: left;
  animation: grow 0.7s var(--ease) backwards;
}

.score__num {
  font-size: 0.72rem;
  color: var(--text-2);
}

@keyframes grow {
  from {
    transform: scaleX(0);
  }
  to {
    transform: scaleX(1);
  }
}
</style>
