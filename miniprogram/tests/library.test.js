const test = require('node:test')
const assert = require('node:assert/strict')

const { filterChunks, groupChunksBySource } = require('../utils/library')
const { formatBytes, shortName } = require('../utils/format')

const chunks = [
  { id: '1', source: 'go.md', content: 'goroutine 调度模型', bytes: 20 },
  { id: '2', source: 'rag.md', content: '混合检索与重排', bytes: 30 },
  { id: '3', source: 'go.md', content: 'channel 通信', bytes: 10 },
]

test('来源聚合会统计片段数与字节并按数量排序', () => {
  assert.deepEqual(groupChunksBySource(chunks), [
    { source: 'go.md', count: 2, bytes: 30, items: [chunks[0], chunks[2]] },
    { source: 'rag.md', count: 1, bytes: 30, items: [chunks[1]] },
  ])
})

test('搜索同时匹配来源名和片段正文且忽略大小写', () => {
  assert.deepEqual(filterChunks(chunks, 'GO'), [chunks[0], chunks[2]])
  assert.deepEqual(filterChunks(chunks, '重排'), [chunks[1]])
  assert.deepEqual(filterChunks(chunks, ''), chunks)
})

test('文件大小和长文件名使用移动端友好格式', () => {
  assert.equal(formatBytes(1536), '1.5 KB')
  assert.equal(formatBytes(0), '0 B')
  assert.equal(shortName('2026年企业级RAG混合检索工程实战.md', 13), '2026年企业级RAG混…')
})
