const test = require('node:test')
const assert = require('node:assert/strict')

const { createSSEParser, createUtf8Decoder } = require('../utils/sse')

test('UTF-8 解码器能跨分块保留中文字符', () => {
  const bytes = new TextEncoder().encode('知识库')
  const decoder = createUtf8Decoder()

  const first = decoder.push(bytes.slice(0, 2))
  const second = decoder.push(bytes.slice(2))

  assert.equal(first, '')
  assert.equal(second, '知识库')
})

test('SSE 解析器能处理被拆开的 JSON 事件', () => {
  const deltas = []
  const references = []
  const parser = createSSEParser({
    onDelta: (value) => deltas.push(value),
    onReferences: (value) => references.push(...value),
  })

  parser.push('data: {"delta":"你')
  parser.push('好"}\n\ndata: {"references":[{"source":"go.md","score":0.9}],"done":true}\n\n')

  assert.deepEqual(deltas, ['你好'])
  assert.deepEqual(references, [{ source: 'go.md', score: 0.9 }])
})

test('SSE 解析器收到结束标记后不再发送事件', () => {
  const deltas = []
  const parser = createSSEParser({ onDelta: (value) => deltas.push(value) })

  parser.push('data: [DONE]\n\ndata: {"delta":"不应出现"}\n\n')

  assert.deepEqual(deltas, [])
  assert.equal(parser.done(), true)
})

test('完成事件和结束标记只触发一次完成回调', () => {
  let completed = 0
  const parser = createSSEParser({ onDone: () => { completed += 1 } })

  parser.push('data: {"references":[],"done":true}\n\ndata: [DONE]\n\n')

  assert.equal(completed, 1)
})

test('done 事件会立即将解析器标记为完成', () => {
  const parser = createSSEParser()

  parser.push('data: {"references":[],"done":true}\n\n')

  assert.equal(parser.done(), true)
})

test('SSE 解析器支持后端五类命名事件', () => {
  const received = []
  const parser = createSSEParser({
    onDelta: (value) => received.push(['delta', value]),
    onReference: (value) => received.push(['reference', value.source_name]),
    onUsage: (value) => received.push(['usage', value.output_tokens]),
    onError: (value) => received.push(['error', value.code]),
    onDone: (value) => received.push(['done', value.message_id]),
  })

  parser.push('event: delta\ndata: {"delta":"回答"}\n\n')
  parser.push('event: reference\ndata: {"source_name":"rag.md"}\n\n')
  parser.push('event: usage\ndata: {"output_tokens":18}\n\n')
  parser.push('event: error\ndata: {"code":"CHAT_FAILED","message":"失败"}\n\n')
  parser.push('event: done\ndata: {"message_id":"m1"}\n\n')

  assert.deepEqual(received, [
    ['delta', '回答'],
    ['reference', 'rag.md'],
    ['usage', 18],
    ['error', 'CHAT_FAILED'],
    ['done', 'm1'],
  ])
})
