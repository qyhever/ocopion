import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { createRequire } from 'node:module'
import vm from 'node:vm'
import { test } from 'node:test'
import ts from 'typescript'
import * as vue from 'vue'
import * as vueRouter from 'vue-router'
import { parse, compileScript } from 'vue/compiler-sfc'

const require = createRequire(import.meta.url)
const source = (path) => readFileSync(new URL(`../src/${path}`, import.meta.url), 'utf8')
function evaluate(code, imports, globals = {}) {
  const output = ts.transpileModule(code.replaceAll('import.meta.env.BASE_URL', "'/'"), {
    compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2022 },
  }).outputText
  const module = { exports: {} }
  vm.runInNewContext(`(function(require,module,exports){${output}\n})`, { console, ...globals })(
    (name) => imports[name] ?? (name.endsWith('.vue') ? {} : require(name)),
    module,
    module.exports,
  )
  return module.exports
}

// 使用 Vue 自定义渲染器执行真实页面，替换网络及 TDesign 外观组件。
function mountPage(speedTest, { writeText = async () => {}, messages = [] } = {}) {
  const { descriptor } = parse(source('views/cloudflare/SpeedTestView.vue'))
  const compiled = compileScript(descriptor, {
    id: 'speed-test',
    inlineTemplate: true,
    templateOptions: { compilerOptions: { hoistStatic: false } },
  })
  const component = evaluate(
    compiled.content,
    {
      vue,
      '@/api/cloudflare': { speedTest },
      'tdesign-vue-next': {
        MessagePlugin: {
          success: (message) => messages.push(['success', message]),
          error: (message) => messages.push(['error', message]),
        },
      },
    },
    { navigator: { clipboard: { writeText } } },
  ).default
  const node = (type, text = '') => ({ type, text, children: [], props: {}, parent: null })
  const renderer = vue.createRenderer({
    createElement: node,
    createText: (text) => node('#text', text),
    createComment: (text) => node('#comment', text),
    setText: (n, text) => {
      n.text = text
    },
    setElementText: (n, text) => {
      n.text = text
      n.children = []
    },
    patchProp: (n, key, _prev, value) => {
      n.props[key] = value
    },
    parentNode: (n) => n.parent,
    nextSibling: (n) => n.parent?.children[n.parent.children.indexOf(n) + 1],
    insert(n, parent, anchor) {
      if (n.parent) n.parent.children.splice(n.parent.children.indexOf(n), 1)
      const index = anchor ? parent.children.indexOf(anchor) : -1
      parent.children.splice(index < 0 ? parent.children.length : index, 0, n)
      n.parent = parent
    },
    remove(n) {
      n.parent?.children.splice(n.parent.children.indexOf(n), 1)
    },
  })
  const root = node('root')
  const app = renderer.createApp(component)
  app.component('t-button', {
    setup:
      (_, { attrs, slots }) =>
      () =>
        vue.h('button', attrs, slots.default?.()),
  })
  app.component('t-alert', {
    props: ['message'],
    setup: (props) => () => vue.h('aside', props.message),
  })
  app.component('t-textarea', {
    setup:
      (_, { attrs }) =>
      () =>
        vue.h('textarea', attrs),
  })
  app.mount(root)
  const find = (type, n = root) => [
    ...(n.type === type ? [n] : []),
    ...n.children.flatMap((child) => find(type, child)),
  ]
  const text = (n = root) =>
    n.type === '#comment' ? '' : n.text + n.children.map((child) => text(child)).join('')
  return { app, find, text }
}

const deferred = () => {
  let resolve, reject
  const promise = new Promise((res, rej) => {
    resolve = res
    reject = rej
  })
  return { promise, resolve, reject }
}

test('页面加载、防重复点击、12 个结果、失败及重试', async () => {
  let calls = 0
  let pending = deferred()
  const page = mountPage(() => {
    calls++
    return pending.promise
  })
  try {
    assert.match(page.text(), /尚无测速结果/)
    const click = () => page.find('button')[0].props.onClick()
    const first = click()
    await vue.nextTick()
    assert.equal(page.find('button')[0].props.disabled, true)
    assert.match(page.text(), /正在测速/)
    await click()
    assert.equal(calls, 1)
    const ips = Array.from({ length: 12 }, (_, i) => `1.1.1.${i + 1}`)
    pending.resolve({ ips })
    await first
    await vue.nextTick()
    assert.equal(page.find('textarea').length, 1)
    assert.equal(page.find('textarea')[0].props.value, ips.join('\n'))
    assert.equal(page.find('textarea')[0].props.readonly, '')
    assert.equal(page.find('button')[0].props.disabled, false)
    pending = deferred()
    const failure = click()
    await vue.nextTick()
    assert.equal(page.find('textarea').length, 0)
    pending.reject(new Error('程序失败'))
    await failure
    await vue.nextTick()
    assert.match(page.text(), /本次测速失败/)
    assert.equal(page.find('button')[0].props.disabled, false)
    pending = deferred()
    const retry = click()
    pending.resolve({ ips })
    await retry
    await vue.nextTick()
    assert.equal(page.find('textarea')[0].props.value, ips.join('\n'))
    assert.equal(calls, 3)
  } finally {
    page.app.unmount()
  }
})

test('复制文本域内容，保留换行并提示成功或失败', async () => {
  const ips = ['1.1.1.1', '1.0.0.1']
  const copied = []
  const messages = []
  let failCopy = false
  const page = mountPage(async () => ({ ips }), {
    messages,
    writeText: async (value) => {
      if (failCopy) throw new Error('剪贴板不可用')
      copied.push(value)
    },
  })
  try {
    const copyButton = () => page.find('button')[1]
    assert.equal(copyButton().props.disabled, true)
    await copyButton().props.onClick()
    assert.equal(copied.length, 0)
    await page.find('button')[0].props.onClick()
    await vue.nextTick()
    assert.equal(copyButton().props.disabled, false)
    await copyButton().props.onClick()
    assert.deepEqual(copied, [page.find('textarea')[0].props.value])
    assert.equal(copied[0], ips.join('\n'))
    assert.deepEqual(messages.at(-1), ['success', '复制成功'])
    failCopy = true
    await copyButton().props.onClick()
    assert.deepEqual(messages.at(-1), ['error', '复制失败，请手动选择文本域内容复制'])
  } finally {
    page.app.unmount()
  }
})

test('API 无请求体，使用统一 post 与 610 秒超时', () => {
  let call
  const api = evaluate(source('api/cloudflare.ts'), {
    '@/utils/request': {
      post: (...args) => {
        call = args
      },
    },
  })
  api.speedTest()
  assert.equal(call[0], '/cloudflare/speed-test')
  assert.equal(call[1], undefined)
  assert.equal(call[2].timeout, 610_000)
})

test('菜单导航及登录守卫沿用独立页面路由', async () => {
  let guard
  const router = evaluate(source('router/index.ts'), {
    'vue-router': {
      ...vueRouter,
      createWebHistory: vueRouter.createMemoryHistory,
      createRouter(options) {
        const router = vueRouter.createRouter(options)
        router.beforeEach = (fn) => {
          guard = fn
        }
        return router
      },
    },
    '@/stores/user': { useUserStore: () => ({ accessToken: '', userInfo: null }) },
  }).default
  const menus = evaluate(source('layouts/components/menu.ts'), {}).menuList
  const menu = menus.find((item) => item.title === 'Cloudflare 优选')
  assert.equal(menu.path, '/cloudflare/speed-test')
  const target = router.resolve(menu.path)
  assert.equal(target.name, 'cloudflare-speed-test')
  assert.equal(target.meta.title, 'Cloudflare 优选')
  assert.equal(target.matched.length, 2)
  const redirect = await guard(target)
  assert.equal(redirect.path, '/signin')
  assert.equal(redirect.query.redirect, menu.path)
})
