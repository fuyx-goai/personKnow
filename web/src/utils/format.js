// format.js —— 展示用的小格式化函数

// bytes 把字节数写成短形式（片段大小用）
export function bytes(n) {
  if (!n) return '0 B'
  if (n < 1024) return `${n} B`
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`
  return `${(n / 1024 / 1024).toFixed(1)} MB`
}

// clock 当前时间 HH:MM:SS（问答记录的时间戳用）
export function clock(date = new Date()) {
  return date.toLocaleTimeString('zh-CN', { hour12: false })
}

// percent 相似度分数 -> 百分比整数
export function percent(score) {
  if (!Number.isFinite(score)) return '—'
  return `${Math.round(score * 100)}%`
}

// greeting 按当下时段说一句问候，让"个人专属"多一分人味
export function greeting(date = new Date()) {
  const h = date.getHours()
  if (h < 5) return '夜深了'
  if (h < 11) return '早安'
  if (h < 14) return '午安'
  if (h < 18) return '下午好'
  return '晚上好'
}
