const REFRESH_TOKEN_KEY = 'personknow.refresh_token'

export function createAuthStore(storage = globalThis.localStorage) {
  let accessToken = ''
  let user = null

  function accept(result = {}) {
    accessToken = result.access_token || ''
    user = result.user || user
    if (result.refresh_token) storage?.setItem(REFRESH_TOKEN_KEY, result.refresh_token)
    return result
  }

  function clear() {
    accessToken = ''
    user = null
    storage?.removeItem(REFRESH_TOKEN_KEY)
  }

  return {
    accept,
    accessToken: () => accessToken,
    clear,
    refreshToken: () => storage?.getItem(REFRESH_TOKEN_KEY) || '',
    user: () => user,
  }
}

export async function pollLoginTicket(options) {
  const ticket = await options.createTicket()
  options.onTicket?.(ticket)
  const sleep = options.sleep || ((duration) => new Promise((resolve) => setTimeout(resolve, duration)))
  const interval = options.interval || 2000
  const maxAttempts = options.maxAttempts || 150

  try {
    for (let attempt = 0; attempt < maxAttempts; attempt += 1) {
      if (options.signal?.aborted) throw new DOMException('Aborted', 'AbortError')
      const result = await options.pollTicket(ticket)
      options.onState?.(result.status || 'pending', ticket)
      if (result.status === 'consumed') return result
      if (result.status === 'expired') throw expiredError()
      await sleep(interval)
    }
  } catch (error) {
    if (error.code === 'TICKET_INVALID') options.onState?.('expired', ticket)
    throw error
  }
  const error = expiredError()
  options.onState?.('expired', ticket)
  throw error
}

function expiredError() {
  const error = new Error('二维码已过期，请刷新重试')
  error.code = 'TICKET_INVALID'
  return error
}

export { REFRESH_TOKEN_KEY }
