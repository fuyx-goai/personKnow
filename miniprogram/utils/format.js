function formatBytes(value = 0) {
  const bytes = Math.max(0, Number(value) || 0)
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${trim(bytes / 1024)} KB`
  return `${trim(bytes / 1024 / 1024)} MB`
}

function trim(value) {
  return Number.isInteger(value) ? String(value) : value.toFixed(1)
}

function shortName(value = '', maxLength = 18) {
  const text = String(value)
  if (text.length <= maxLength) return text
  return `${text.slice(0, Math.max(1, maxLength - 1))}…`
}

function formatScore(value = 0) {
  return `${Math.round(Math.max(0, Math.min(1, Number(value) || 0)) * 100)}%`
}

module.exports = { formatBytes, formatScore, shortName }
