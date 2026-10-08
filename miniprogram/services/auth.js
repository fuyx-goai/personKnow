const REFRESH_TOKEN_KEY = 'refreshToken'

function defaultStorage() {
  return typeof wx === 'undefined' ? {} : wx
}

function createAuthSession(options = {}) {
  const storage = options.storage || defaultStorage()
  let accessToken = ''
  let currentUser = null
  let refreshPromise = null

  function clear() {
    accessToken = ''
    currentUser = null
    storage.removeStorageSync?.(REFRESH_TOKEN_KEY)
  }

  function accept(result = {}) {
    accessToken = result.access_token || ''
    currentUser = result.user || currentUser
    if (result.refresh_token) storage.setStorageSync?.(REFRESH_TOKEN_KEY, result.refresh_token)
    return result
  }

  async function login(profile = {}) {
    if (!options.login || !options.exchange) throw new Error('微信登录尚未配置')
    const loginResult = await options.login()
    if (!loginResult?.code) throw new Error('未获取到微信登录 code')
    return accept(await options.exchange({ ...profile, code: loginResult.code }))
  }

  async function refresh() {
    if (refreshPromise) return refreshPromise
    const refreshToken = storage.getStorageSync?.(REFRESH_TOKEN_KEY)
    if (!refreshToken || !options.refresh) {
      clear()
      throw new Error('SESSION_INVALID')
    }
    refreshPromise = options.refresh(refreshToken).then(accept).catch((error) => {
      clear()
      throw error
    }).finally(() => { refreshPromise = null })
    return refreshPromise
  }

  async function restore(profile = {}) {
    if (storage.getStorageSync?.(REFRESH_TOKEN_KEY)) {
      try { return await refresh() } catch (_) { clear() }
    }
    return login(profile)
  }

  async function logout() {
    try {
      if (accessToken && options.revoke) await options.revoke(accessToken)
    } finally {
      clear()
    }
  }

  return {
    accept,
    accessToken: () => accessToken,
    clear,
    login,
    logout,
    refresh,
    restore,
    user: () => currentUser,
  }
}

function parseWebLoginPayload(payload = '') {
  const prefix = 'personknow://web-login?'
  if (!String(payload).startsWith(prefix)) throw new Error('无效的 Web 登录二维码')
  const values = {}
  String(payload).slice(prefix.length).split('&').forEach((part) => {
    const [key, value] = part.split('=')
    if (key && value) values[decodeURIComponent(key)] = decodeURIComponent(value)
  })
  if (!values.id || !values.secret) throw new Error('无效的 Web 登录票据')
  return { id: values.id, secret: values.secret }
}

module.exports = { REFRESH_TOKEN_KEY, createAuthSession, parseWebLoginPayload }
