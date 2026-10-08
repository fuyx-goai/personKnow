function expectedBytes(first) {
  if (first < 0x80) return 1
  if ((first & 0xe0) === 0xc0) return 2
  if ((first & 0xf0) === 0xe0) return 3
  if ((first & 0xf8) === 0xf0) return 4
  return 1
}

function completeByteLength(bytes) {
  if (!bytes.length) return 0
  let leadIndex = bytes.length - 1
  while (leadIndex >= 0 && (bytes[leadIndex] & 0xc0) === 0x80) leadIndex -= 1
  if (leadIndex < 0) return 0
  const available = bytes.length - leadIndex
  return available < expectedBytes(bytes[leadIndex]) ? leadIndex : bytes.length
}

function decodeBytes(bytes) {
  let output = ''
  for (let index = 0; index < bytes.length;) {
    const first = bytes[index]
    const size = expectedBytes(first)
    if (size === 1) {
      output += String.fromCodePoint(first)
      index += 1
      continue
    }
    let codePoint = first & (size === 2 ? 0x1f : size === 3 ? 0x0f : 0x07)
    for (let offset = 1; offset < size; offset += 1) {
      codePoint = (codePoint << 6) | (bytes[index + offset] & 0x3f)
    }
    output += String.fromCodePoint(codePoint)
    index += size
  }
  return output
}

function createUtf8Decoder() {
  let pending = new Uint8Array(0)
  return {
    push(input, flush = false) {
      const incoming = input instanceof Uint8Array ? input : new Uint8Array(input)
      const bytes = new Uint8Array(pending.length + incoming.length)
      bytes.set(pending)
      bytes.set(incoming, pending.length)
      const length = flush ? bytes.length : completeByteLength(bytes)
      pending = bytes.slice(length)
      return decodeBytes(bytes.slice(0, length))
    },
  }
}

function createSSEParser(handlers = {}) {
  let buffer = ''
  let finished = false
  let notified = false

  function notifyDone() {
    if (notified) return
    notified = true
    handlers.onDone?.()
  }

  function emit(block) {
    const payload = block.split('\n')
      .filter((line) => line.startsWith('data:'))
      .map((line) => line.slice(5).trim())
      .join('\n')
    if (!payload) return
    if (payload === '[DONE]') {
      finished = true
      notifyDone()
      return
    }
    try {
      const event = JSON.parse(payload)
      if (event.error) handlers.onError?.(event.error)
      if (event.delta) handlers.onDelta?.(event.delta)
      if (event.references) handlers.onReferences?.(event.references)
      if (event.done) notifyDone()
    } catch (error) {
      handlers.onError?.(`响应解析失败：${error.message}`)
    }
  }

  return {
    push(text) {
      if (finished) return
      buffer += text.replace(/\r\n/g, '\n')
      let boundary = buffer.indexOf('\n\n')
      while (boundary >= 0 && !finished) {
        emit(buffer.slice(0, boundary))
        buffer = buffer.slice(boundary + 2)
        boundary = buffer.indexOf('\n\n')
      }
    },
    done() { return finished || notified },
  }
}

module.exports = { createSSEParser, createUtf8Decoder }
