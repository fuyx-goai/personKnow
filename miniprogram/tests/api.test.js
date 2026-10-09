const test = require('node:test')
const assert = require('node:assert/strict')

const { ApiError, createAPIClient, createAuthAPI, pollIndexJob } = require('../services/api')
const { createUploader } = require('../services/upload')
const { canManageLibrary } = require('../utils/library')
const silentLogger = { info() {} }

test('遇到 401 时刷新令牌并且只重试一次', async () => {
  const headers = []
  let refreshed = 0
  const auth = {
    token: 'old-token',
    accessToken() { return this.token },
    async refresh() { refreshed += 1; this.token = 'new-token' },
  }
  const client = createAPIClient({
    auth,
    logger: silentLogger,
    baseUrl: () => 'https://api.example.com',
    transport: async (options) => {
      headers.push(options.header.Authorization)
      if (headers.length === 1) return { statusCode: 401, data: { code: 'SESSION_INVALID' } }
      return { statusCode: 200, data: { owned: [], public: [] } }
    },
  })

  await client.libraries()

  assert.equal(refreshed, 1)
  assert.deepEqual(headers, ['Bearer old-token', 'Bearer new-token'])
})

test('刷新后仍为 401 时不会重复重试', async () => {
  let requests = 0
  const client = createAPIClient({
    auth: { accessToken: () => 'token', refresh: async () => {} },
    logger: silentLogger,
    baseUrl: () => 'https://api.example.com',
    transport: async () => {
      requests += 1
      return { statusCode: 401, data: { code: 'SESSION_INVALID', message: '登录已失效' } }
    },
  })

  await assert.rejects(() => client.libraries(), (error) => {
    assert.equal(error.code, 'SESSION_INVALID')
    return true
  })
  assert.equal(requests, 2)
})

test('微信登录把 code 发送到完整的 v1 登录地址', async () => {
  let captured
  const authAPI = createAuthAPI({
    baseUrl: () => 'http://127.0.0.1:8080/',
    logger: silentLogger,
    transport: async (options) => {
      captured = options
      return { statusCode: 200, data: { access_token: 'access-token' } }
    },
  })

  await authAPI.exchange({ code: 'wx-code', device_label: '微信小程序' })

  assert.equal(captured.url, 'http://127.0.0.1:8080/api/v1/auth/wechat/login')
  assert.equal(captured.method, 'POST')
  assert.deepEqual(captured.data, { code: 'wx-code', device_label: '微信小程序' })
})

test('启动探针访问公开的全栈状态接口', async () => {
  let captured
  const client = createAPIClient({
    baseUrl: () => 'http://127.0.0.1:8080',
    logger: silentLogger,
    transport: async (options) => {
      captured = options
      return { statusCode: 200, data: { version: 'v1', capabilities: ['miniprogram'] } }
    },
  })

  const result = await client.status()

  assert.equal(captured.url, 'http://127.0.0.1:8080/api/v1/status')
  assert.equal(captured.header.Authorization, undefined)
  assert.equal(result.capabilities.includes('miniprogram'), true)
})

test('旧网关返回纯文本 404 时提示重启最新后端', async () => {
  const authAPI = createAuthAPI({
    baseUrl: () => 'http://127.0.0.1:8080',
    logger: silentLogger,
    transport: async () => ({ statusCode: 404, data: '404 page not found' }),
  })

  await assert.rejects(() => authAPI.exchange({ code: 'wx-code' }), (error) => {
    assert.ok(error instanceof ApiError)
    assert.equal(error.code, 'API_VERSION_MISMATCH')
    assert.match(error.message, /重启最新 Go 网关/)
    return true
  })
})

test('配额错误保留服务端错误码与 request_id', async () => {
  const client = createAPIClient({
    auth: { accessToken: () => 'token' },
    logger: silentLogger,
    baseUrl: () => 'https://api.example.com',
    transport: async () => ({
      statusCode: 507,
      data: { code: 'STORAGE_QUOTA_EXCEEDED', message: '存储空间已达上限', request_id: 'req-9' },
    }),
  })

  await assert.rejects(() => client.documents(), (error) => {
    assert.ok(error instanceof ApiError)
    assert.equal(error.code, 'STORAGE_QUOTA_EXCEEDED')
    assert.equal(error.requestId, 'req-9')
    return true
  })
})

test('接口日志记录路径与状态但不泄露令牌', async () => {
  const entries = []
  const client = createAPIClient({
    auth: { accessToken: () => 'super-secret-token' },
    baseUrl: () => 'https://api.example.com',
    logger: { info: (message, fields) => entries.push({ message, fields }) },
    transport: async () => ({
      statusCode: 200,
      data: { owned: [], public: [] },
      header: { 'X-Request-ID': 'req-12' },
    }),
  })

  await client.libraries()

  assert.equal(entries.length, 2)
  assert.equal(entries[0].fields.path, '/api/v1/libraries')
  assert.equal(entries[1].fields.status_code, 200)
  assert.equal(entries[1].fields.request_id, 'req-12')
  assert.equal(JSON.stringify(entries).includes('super-secret-token'), false)
})

test('索引任务轮询在 ready 终态停止', async () => {
  const statuses = ['running', 'running', 'ready']
  let sleeps = 0

  const result = await pollIndexJob(
    async () => ({ status: statuses.shift() }),
    { sleep: async () => { sleeps += 1 }, interval: 1 },
  )

  assert.equal(result.status, 'ready')
  assert.equal(sleeps, 2)
})

test('公开知识库保持只读且仅所有者可管理', () => {
  assert.equal(canManageLibrary({ access: 'owner' }), true)
  assert.equal(canManageLibrary({ access: 'read', visibility: 'public' }), false)
})

test('文件上传携带知识库与访问令牌', async () => {
  let captured
  const uploader = createUploader({
    auth: { accessToken: () => 'access-token' },
    logger: silentLogger,
    baseUrl: () => 'https://api.example.com',
    uploadFile: async (options) => {
      captured = options
      return { statusCode: 202, data: JSON.stringify({ document: { id: 'd1' }, job_id: 'j1' }) }
    },
  })

  const result = await uploader.uploadDocument('/tmp/rag.md', 'library-1', 'rag.md')

  assert.equal(result.job_id, 'j1')
  assert.equal(captured.formData.library_id, 'library-1')
  assert.equal(captured.header.Authorization, 'Bearer access-token')
  assert.equal(captured.name, 'file')
  assert.equal(captured.url, 'https://api.example.com/api/v1/documents')
})

test('文件上传会清理服务地址末尾斜杠', async () => {
  let captured
  const uploader = createUploader({
    auth: { accessToken: () => 'access-token' },
    logger: silentLogger,
    baseUrl: () => 'https://api.example.com/',
    uploadFile: async (options) => {
      captured = options
      return { statusCode: 202, data: '{}' }
    },
  })

  await uploader.uploadDocument('/tmp/rag.md', 'library-1', 'rag.md')

  assert.equal(captured.url, 'https://api.example.com/api/v1/documents')
})
