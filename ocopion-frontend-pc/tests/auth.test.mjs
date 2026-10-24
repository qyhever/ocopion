import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { createRequire } from 'node:module'
import path from 'node:path'
import { fileURLToPath } from 'node:url'
import vm from 'node:vm'
import { test } from 'node:test'
import ts from 'typescript'
import * as vue from 'vue'
import * as pinia from 'pinia'
import * as vueRouter from 'vue-router'

const root = fileURLToPath(new URL('../', import.meta.url))
const nativeRequire = createRequire(import.meta.url)
const tokens = { accessToken: 'access-old', refreshToken: 'refresh-old' }
const fresh = { accessToken: 'access-new', refreshToken: 'refresh-new' }
const user = { id: 1, email: 'user@example.com', username: 'tester', nickname: '测试用户' }
const ok = (data) =>
  new Response(JSON.stringify({ code: 200, message: '操作成功', data }), {
    headers: { 'Content-Type': 'application/json' },
  })
const fail = (status = 401, message = '认证失败') =>
  new Response(JSON.stringify({ code: status === 401 ? 1006 : -1, message }), {
    status,
    headers: { 'Content-Type': 'application/json' },
  })
const deferred = () => {
  let resolve
  const promise = new Promise((r) => {
    resolve = r
  })
  return { promise, resolve }
}

// 执行实际 TS 模块，仅替换 UI 提示、浏览器存储和 HTTP 传输。
export function createHarness(handler, initial = tokens) {
  pinia.setActivePinia(pinia.createPinia())
  const storage = new Map(Object.entries(initial))
  const messages = []
  const calls = []
  const navigations = []
  let guard
  let router
  const context = vm.createContext({
    console,
    URL,
    URLSearchParams,
    FormData,
    Blob,
    AbortController,
    setTimeout,
    clearTimeout,
    window: { location: { origin: 'http://localhost:5175' } },
    localStorage: {
      getItem: (key) => storage.get(key) ?? null,
      setItem: (key, value) => storage.set(key, value),
      removeItem: (key) => storage.delete(key),
    },
    fetch: async (url, options) => {
      const call = { url: new URL(url), ...options }
      calls.push(call)
      return handler(call)
    },
  })
  const cache = new Map()
  function load(filename) {
    if (filename.endsWith('.vue')) return { default: {}, __esModule: true }
    if (cache.has(filename)) return cache.get(filename).exports
    const module = { exports: {} }
    cache.set(filename, module)
    const source = readFileSync(filename, 'utf8').replaceAll('import.meta.env.BASE_URL', "'/'")
    const { outputText } = ts.transpileModule(source, {
      compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2022 },
    })
    const require = (specifier) => {
      if (specifier === 'vue') return vue
      if (specifier === 'pinia') return pinia
      if (specifier === 'tdesign-vue-next')
        return {
          MessagePlugin: Object.fromEntries(
            ['error', 'warning', 'success'].map((type) => [
              type,
              (message) => messages.push({ type, message }),
            ]),
          ),
        }
      if (specifier === 'vue-router')
        return {
          ...vueRouter,
          createWebHistory: vueRouter.createMemoryHistory,
          createRouter(options) {
            router = vueRouter.createRouter(options)
            router.beforeEach = (fn) => {
              guard = fn
            }
            router.replace = async (to) => {
              navigations.push(to)
            }
            return router
          },
        }
      if (specifier.startsWith('@/') || specifier.startsWith('.')) {
        let target = specifier.startsWith('@/')
          ? path.join(root, 'src', specifier.slice(2))
          : path.resolve(path.dirname(filename), specifier)
        if (!path.extname(target)) target += target.endsWith('/router') ? '/index.ts' : '.ts'
        return load(target)
      }
      return nativeRequire(specifier)
    }
    vm.runInContext(`(function(require, module, exports) {${outputText}\n})`, context, {
      filename,
    })(require, module, module.exports)
    return module.exports
  }
  const api = load(path.join(root, 'src/utils/request.ts'))
  const store = load(path.join(root, 'src/stores/user.ts')).useUserStore()
  const routes = load(path.join(root, 'src/router/index.ts'))
  return {
    api,
    store,
    routes,
    storage,
    calls,
    messages,
    navigations,
    guard: (path, query = {}) => guard({ path, fullPath: path, query }),
    router,
  }
}

test('邮箱登录保存 Token 对并加载 /user/me，退出清理存储', async () => {
  const h = createHarness(
    (c) => (c.url.pathname.endsWith('/auth/login') ? ok(tokens) : ok(user)),
    {},
  )
  await h.store.loginByEmail('user@example.com', 'x')
  assert.deepEqual(JSON.parse(h.calls[0].body), { email: 'user@example.com', password: 'x' })
  assert.equal(h.calls[0].headers.Authorization, undefined)
  assert.equal(h.calls[1].url.pathname, '/ocopion/api/user/me')
  assert.equal(h.calls[1].headers.Authorization, 'Bearer access-old')
  assert.equal(h.store.userInfo.id, 1)
  assert.equal(h.store.displayName, '测试用户')
  assert.equal(h.storage.get('refreshToken'), tokens.refreshToken)
  h.store.logout()
  assert.equal(h.storage.size, 0)
  assert.equal(h.store.userInfo, null)
})

test('错误密码保留后端业务错误，仅提示一次且不刷新', async () => {
  const h = createHarness(() => fail(200, '邮箱或密码错误'), {})
  await assert.rejects(h.store.loginByEmail('user@example.com', 'bad'), /邮箱或密码错误/)
  assert.equal(h.calls.length, 1)
  assert.deepEqual(h.messages, [{ type: 'warning', message: '邮箱或密码错误' }])
  assert.equal(h.storage.size, 0)
})

test('登录后用户信息失败，清理本次会话', async () => {
  const h = createHarness(
    (c) => (c.url.pathname.endsWith('/auth/login') ? ok(tokens) : fail(500, '服务异常')),
    {},
  )
  await assert.rejects(h.store.loginByEmail('user@example.com', 'x'), /服务异常/)
  assert.equal(h.storage.size, 0)
  assert.equal(h.messages.length, 1)
})

test('并发 401 只刷新一次，保存新 Token 并各重试一次', async () => {
  const release = deferred()
  const started = deferred()
  const h = createHarness(async (c) => {
    if (c.url.pathname.endsWith('/auth/refresh')) {
      started.resolve()
      await release.promise
      assert.deepEqual(JSON.parse(c.body), { refreshToken: 'refresh-old' })
      assert.equal(c.headers.Authorization, undefined)
      return ok(fresh)
    }
    return c.headers.Authorization === 'Bearer access-new' ? ok(user) : fail()
  })
  const requests = Promise.all([h.api.get('/user/me'), h.api.get('/user/me')])
  await started.promise
  release.resolve()
  const result = await requests
  assert.equal(result[0].id, 1)
  assert.equal(h.calls.filter((c) => c.url.pathname.endsWith('/auth/refresh')).length, 1)
  assert.equal(h.calls.length, 5)
  assert.equal(h.storage.get('accessToken'), fresh.accessToken)
  assert.equal(h.storage.get('refreshToken'), fresh.refreshToken)
  assert.equal(h.messages.length, 0)
})

test('较晚返回的旧 Token 401 复用新 Token，不重复刷新', async () => {
  const late = deferred()
  const h = createHarness(async (c) => {
    if (c.url.pathname.endsWith('/auth/refresh')) return ok(fresh)
    if (c.headers.Authorization === 'Bearer access-new') return ok(user)
    if (c.url.pathname.endsWith('/late')) await late.promise
    return fail()
  })
  const pending = h.api.get('/late')
  await h.api.get('/user/me')
  late.resolve()
  await pending
  assert.equal(h.calls.filter((c) => c.url.pathname.endsWith('/auth/refresh')).length, 1)
})

for (const mode of [
  '无效刷新 Token',
  '缺失刷新 Token',
  '刷新网络失败',
  '刷新业务失败',
  '重试仍然 401',
]) {
  test(`${mode}：退出登录，提示一次，无刷新循环`, async () => {
    const h = createHarness(
      (c) => {
        if (!c.url.pathname.endsWith('/auth/refresh')) return fail()
        if (mode === '刷新网络失败') throw new Error('网络错误')
        if (mode === '刷新业务失败') return fail(200, '刷新失败')
        return mode === '重试仍然 401' ? ok(fresh) : fail()
      },
      mode === '缺失刷新 Token' ? { accessToken: tokens.accessToken } : tokens,
    )
    const result = await Promise.allSettled([h.api.get('/user/me'), h.api.get('/user/me')])
    assert.ok(result.every((r) => r.status === 'rejected'))
    assert.equal(h.storage.size, 0)
    assert.equal(h.messages.length, 1)
    assert.equal(h.navigations.length, 1)
    assert.equal(h.navigations[0].path, '/signin')
    const refreshes = h.calls.filter((c) => c.url.pathname.endsWith('/auth/refresh')).length
    assert.equal(refreshes, mode === '缺失刷新 Token' ? 0 : 1)
    assert.ok(h.calls.length <= 5)
  })
}

test('登录接口 401 不自动刷新，保留错误信息', async () => {
  const h = createHarness(() => fail(401, '登录被拒绝'))
  await assert.rejects(h.api.post('/auth/login', {}), /登录被拒绝/)
  assert.equal(h.calls.length, 1)
  assert.equal(h.messages[0].message, '登录被拒绝')
})

test('普通请求网络错误不刷新，守卫保留 Token 并中止导航', async () => {
  const h = createHarness(() => {
    throw new Error('网络不可用')
  })
  assert.equal(await h.guard('/home'), false)
  assert.equal(h.calls.length, 1)
  assert.equal(h.store.accessToken, tokens.accessToken)
  assert.equal(h.messages[0].message, '网络不可用')
})

test('刷新页面恢复用户信息；未登录导航保留原目标', async () => {
  const h = createHarness(() => ok(user))
  assert.equal(await h.guard('/home'), true)
  assert.equal(h.store.userInfo.id, user.id)
  assert.equal(await h.guard('/home'), true)
  assert.equal(h.calls.length, 1)
  h.store.logout()
  const result = await h.guard('/attach/simple-upload')
  assert.equal(result.path, '/signin')
  assert.equal(result.query.redirect, '/attach/simple-upload')
})

test('守卫捕获认证失败并返回登录页', async () => {
  const h = createHarness(() => fail(), { accessToken: tokens.accessToken })
  assert.equal((await h.guard('/home')).path, '/signin')
})

test('只接受有效站内 redirect', () => {
  const h = createHarness(() => ok(user))
  for (const value of [
    'https://example.com',
    '//example.com',
    '/\\example.com',
    '/signin',
    '/signin/',
    '/SIGNIN',
    '/missing',
    ['/home'],
    null,
  ]) {
    assert.equal(h.routes.getLoginRedirect(value), '/home')
  }
  assert.equal(
    h.routes.getLoginRedirect('/attach/simple-upload?tab=1#upload'),
    '/attach/simple-upload?tab=1#upload',
  )
})

test('退出后迟到的刷新响应不能恢复会话', async () => {
  const started = deferred()
  const release = deferred()
  const h = createHarness(async (c) => {
    if (c.url.pathname.endsWith('/auth/refresh')) {
      started.resolve()
      await release.promise
      return ok(fresh)
    }
    return fail()
  })
  const pending = h.api.get('/user/me')
  await started.promise
  h.store.logout()
  release.resolve()
  await assert.rejects(pending)
  assert.equal(h.storage.size, 0)
  assert.equal(h.messages.length, 0)
})

test(
  '真实代理联调：登录、用户信息、刷新恢复和错误密码',
  {
    skip: process.env.AUTH_INTEGRATION !== '1',
  },
  async () => {
    const email = process.env.AUTH_TEST_EMAIL
    const password = process.env.AUTH_TEST_PASSWORD
    assert.ok(email && password, '需要已有测试账号 AUTH_TEST_EMAIL 和 AUTH_TEST_PASSWORD')
    const origin = process.env.AUTH_TEST_ORIGIN || 'http://127.0.0.1:5175'
    const h = createHarness((c) => {
      const { url, ...options } = c
      return fetch(new URL(url.pathname + url.search, origin), options)
    }, {})
    await h.store.loginByEmail(email, password)
    assert.equal(h.store.userInfo.email, email)
    const originalRefresh = h.store.refreshToken
    h.store.setTokens({ accessToken: 'invalid-access-token', refreshToken: originalRefresh })
    const results = await Promise.all([h.api.get('/user/me'), h.api.get('/user/me')])
    assert.equal(results[0].id, h.store.userInfo.id)
    assert.equal(results[1].id, h.store.userInfo.id)
    assert.notEqual(h.store.accessToken, 'invalid-access-token')
    assert.notEqual(h.store.refreshToken, originalRefresh)
    assert.equal(h.calls.filter((c) => c.url.pathname.endsWith('/auth/refresh')).length, 1)
    h.store.userInfo = null
    assert.equal(await h.guard('/home'), true)
    assert.equal(h.store.userInfo.email, email)
    h.store.logout()
    assert.equal(h.storage.size, 0)
    await assert.rejects(
      h.store.loginByEmail(email, 'intentionally-wrong-password'),
      /邮箱或密码错误/,
    )
    assert.equal(h.messages.length, 1)
    assert.equal(h.messages[0].message, '邮箱或密码错误')
    assert.ok(h.calls.every((c) => c.url.pathname !== '/ocopion/api/user/info'))
  },
)
