const test = require('node:test')
const assert = require('node:assert/strict')

const { orderMessages, mapReference } = require('../utils/chat-display')

test('历史消息按问题在前回答在后的时间顺序展示', () => {
  const messages = orderMessages([
    { id: 'answer', role: 'assistant', created_at: '2026-10-12T10:02:00Z' },
    { id: 'question', role: 'user', created_at: '2026-10-12T10:01:00Z' },
  ])

  assert.deepEqual(messages.map((item) => item.id), ['question', 'answer'])
})

test('资料引用卡片保留文档 ID 用于直接查看', () => {
  const reference = mapReference({
    document_id: 'document-1',
    source_name: 'Go 并发编程实践.md',
    excerpt: '使用 context 传递取消信号。',
    similarity: 0.92,
  })

  assert.equal(reference.documentID, 'document-1')
  assert.equal(reference.source, 'Go 并发编程实践.md')
assert.equal(reference.scoreLabel, '92%')
})
