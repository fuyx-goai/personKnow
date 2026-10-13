const { formatScore } = require('./format')

function timestamp(value) {
  const parsed = new Date(value || 0).getTime()
  return Number.isFinite(parsed) ? parsed : 0
}

// 历史接口按倒序分页返回；页面需要恢复成自然阅读顺序，并在同一时间点优先展示问题。
function orderMessages(messages = []) {
  return messages.slice().sort((left, right) => {
    const timeDifference = timestamp(left.created_at) - timestamp(right.created_at)
    if (timeDifference) return timeDifference
    if (left.role === right.role) return String(left.id || '').localeCompare(String(right.id || ''))
    return left.role === 'user' ? -1 : 1
  })
}

// 引用卡片保留 document_id，供页面直接打开对应原资料，而不是只展示静态摘要。
function mapReference(reference = {}) {
  return {
    ...reference,
    documentID: reference.document_id || '',
    source: reference.source_name || reference.source || '原资料',
    snippet: reference.excerpt || reference.snippet || '',
    scoreLabel: formatScore(reference.similarity ?? reference.score),
  }
}

module.exports = { mapReference, orderMessages }
