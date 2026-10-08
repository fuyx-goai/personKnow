const test = require('node:test')
const assert = require('node:assert/strict')

const { createAuthSession } = require('../services/auth')

function createStorage() {
  const values = new Map()
  return {
    getStorageSync: (key) => values.get(key),
    setStorageSync: (key, value) => values.set(key, value),
    removeStorageSync: (key) => values.delete(key),
    values,
  }
}

test('登录仅持久化刷新令牌并将访问令牌保留在内存', async () => {
  const storage = createStorage()
  const session = createAuthSession({
    storage,
    login: async () => ({ code: 'wx-code' }),
    exchange: async (payload) => ({
      access_token: `access-${payload.code}`,
      refresh_token: 'refresh-token',
      user: { id: 'u1', nickname: '林智深' },
    }),
  })

  const result = await session.login({ nickname: '林智深' })

  assert.equal(result.user.nickname, '林智深')
  assert.equal(session.accessToken(), 'access-wx-code')
  assert.equal(storage.values.get('refreshToken'), 'refresh-token')
  assert.equal(storage.values.has('accessToken'), false)
})

test('刷新失败会清空本地会话', async () => {
  const storage = createStorage()
  storage.setStorageSync('refreshToken', 'expired')
  const session = createAuthSession({
    storage,
    refresh: async () => { throw new Error('SESSION_INVALID') },
  })

  await assert.rejects(() => session.refresh(), /SESSION_INVALID/)

  assert.equal(session.accessToken(), '')
  assert.equal(storage.values.has('refreshToken'), false)
})

test('退出登录会撤销服务端会话并清除全部令牌', async () => {
  const storage = createStorage()
  let revoked = ''
  const session = createAuthSession({ storage, revoke: async (token) => { revoked = token } })
  session.accept({ access_token: 'access-token', refresh_token: 'refresh-token' })

  await session.logout()

  assert.equal(revoked, 'access-token')
  assert.equal(session.accessToken(), '')
  assert.equal(storage.values.has('refreshToken'), false)
})
