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

module.exports = { REFRESH_TOKEN_KEY, createAuthSession }
