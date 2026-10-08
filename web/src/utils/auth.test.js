import { describe, expect, it } from 'vitest'

import { ApiError, createAPIClient } from '../api'
import { createAuthStore, pollLoginTicket } from './auth'

function memoryStorage() {
  const values = new Map()
  return {
    getItem: (key) => values.get(key) ?? null,
    setItem: (key, value) => values.set(key, value),
    removeItem: (key) => values.delete(key),
    values,
  }
}

describe('Web 登录会话', () => {
  it('仅持久化刷新令牌并把访问令牌留在内存', () => {
    const storage = memoryStorage()
    const auth = createAuthStore(storage)

    auth.accept({ access_token: 'access', refresh_token: 'refresh', user: { id: 'u1' } })

    expect(auth.accessToken()).toBe('access')
    expect(storage.values.get('personknow.refresh_token')).toBe('refresh')
    expect(storage.values.has('personknow.access_token')).toBe(false)
  })

  it('扫码轮询从 pending 进入 consumed 并返回登录结果', async () => {
    const states = []
    let issuedTicket
    const responses = [{ status: 'pending' }, { status: 'consumed', access_token: 'access' }]

    const result = await pollLoginTicket({
      createTicket: async () => ({ id: 't1', secret: 'secret', qr_payload: 'personknow://login' }),
      pollTicket: async () => responses.shift(),
      sleep: async () => {},
      onState: (state) => states.push(state),
      onTicket: (ticket) => { issuedTicket = ticket },
    })

    expect(result.access_token).toBe('access')
    expect(issuedTicket.id).toBe('t1')
    expect(states).toEqual(['pending', 'consumed'])
  })

  it('失效票据转换为 expired 状态', async () => {
    const states = []
    await expect(pollLoginTicket({
      createTicket: async () => ({ id: 't1', secret: 'secret' }),
      pollTicket: async () => { throw new ApiError({ code: 'TICKET_INVALID', message: '已过期' }, 400) },
      onState: (state) => states.push(state),
    })).rejects.toMatchObject({ code: 'TICKET_INVALID' })

    expect(states).toEqual(['expired'])
  })
})

describe('授权请求', () => {
  it('401 时刷新令牌并且只重试一次', async () => {
    const storage = memoryStorage()
    const auth = createAuthStore(storage)
    auth.accept({ access_token: 'old', refresh_token: 'refresh' })
    const headers = []
    const client = createAPIClient({
      auth,
      fetcher: async (_path, options) => {
        headers.push(options.headers.Authorization)
        return headers.length === 1
          ? new Response(JSON.stringify({ code: 'SESSION_INVALID' }), { status: 401 })
          : new Response(JSON.stringify({ owned: [], public: [] }), { status: 200 })
      },
      refresh: async () => ({ access_token: 'new', refresh_token: 'rotated' }),
    })

    await client.libraries()

    expect(headers).toEqual(['Bearer old', 'Bearer new'])
  })

  it('刷新后仍为 401 时停止重试', async () => {
    const auth = createAuthStore(memoryStorage())
    auth.accept({ access_token: 'old', refresh_token: 'refresh' })
    let requests = 0
    const client = createAPIClient({
      auth,
      fetcher: async () => {
        requests += 1
        return new Response(JSON.stringify({ code: 'SESSION_INVALID', message: '登录失效' }), { status: 401 })
      },
      refresh: async () => ({ access_token: 'new', refresh_token: 'rotated' }),
    })

    await expect(client.libraries()).rejects.toMatchObject({ code: 'SESSION_INVALID' })
    expect(requests).toBe(2)
  })
})
