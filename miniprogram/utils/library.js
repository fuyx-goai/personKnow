function groupChunksBySource(chunks = []) {
  const groups = new Map()
  chunks.forEach((chunk) => {
    const source = chunk.source || '未命名来源'
    if (!groups.has(source)) groups.set(source, { source, count: 0, bytes: 0, items: [] })
    const group = groups.get(source)
    group.count += 1
    group.bytes += Number(chunk.bytes) || 0
    group.items.push(chunk)
  })
  return [...groups.values()].sort((left, right) => right.count - left.count || left.source.localeCompare(right.source))
}

function filterChunks(chunks = [], keyword = '') {
  const query = keyword.trim().toLowerCase()
  if (!query) return chunks
  return chunks.filter((chunk) => {
    const source = String(chunk.source || '').toLowerCase()
    const content = String(chunk.content || '').toLowerCase()
    return source.includes(query) || content.includes(query)
  })
}

module.exports = { filterChunks, groupChunksBySource }
