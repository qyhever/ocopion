import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import vm from 'node:vm'
import { test } from 'node:test'
import ts from 'typescript'

function harness(api = {}, saved = null) {
  const mounted = [], unmounted = [], timers = new Map(), storage = new Map()
  const key = 'txt-video:last-task:42'
  if (saved) storage.set(key, saved)
  const store = { userInfo: { id: 42 }, sessionVersion: 1 }
  const messages = [], revoked = []
  const source = readFileSync(new URL('../src/views/txt-video/TxtVideoView.vue', import.meta.url), 'utf8')
    .match(/<script setup lang="ts">([\s\S]*?)<\/script>/)[1]
  const { outputText } = ts.transpileModule(source + '\nexport { text, speed, cover, task, busy, submitting, queryError, notice, submit, poll, selectCover, fullVideoUrl };', {
    compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2022 },
  })
  const module = { exports: {} }
  const imports = {
    vue: { ref: (value) => ({ value }), computed: (fn) => ({ get value() { return fn() } }), onMounted: (fn) => mounted.push(fn), onUnmounted: (fn) => unmounted.push(fn) },
    'tdesign-vue-next': { MessagePlugin: { warning: (m) => messages.push(m), success: (m) => messages.push(m) } },
    '@/stores/user': { useUserStore: () => store },
    './service': { createVideoTask: async () => ({ id: 'new-task', status: 'running', error: '' }), ...api },
  }
  class FakeURL extends URL {
    static createObjectURL() { return 'blob:preview' }
    static revokeObjectURL(value) { revoked.push(value) }
  }
  vm.runInNewContext(outputText, {
    module, exports: module.exports, require: (name) => imports[name], URL: FakeURL,
    window: { location: { origin: 'https://app.test' } },
    localStorage: { getItem: (k) => storage.get(k) ?? null, setItem: (k, v) => storage.set(k, v), removeItem: (k) => storage.delete(k) },
    setTimeout: (fn) => { const id = Symbol(); timers.set(id, fn); return id }, clearTimeout: (id) => timers.delete(id),
  })
  return { ...module.exports, store, mounted, unmounted, timers, storage, key, messages, revoked }
}

test('提交保留用户任务，阻止重复提交并轮询到成功', async () => {
  let calls = 0
  const h = harness({ createVideoTask: async (...args) => {
    calls++; assert.equal(args[0], '文案'); assert.equal(args[2], '0.5')
    return { id: 'new-task', status: 'running', error: '' }
  }, getVideoTask: async () => ({ id: 'new-task', status: 'succeeded', error: '', videoUrl: '/ocopion/api/txt-video/files/example.mp4' }) })
  h.text.value = ' 文案 '; h.cover.value = new File(['x'], 'cover.png'); h.speed.value = '0.5'
  const pending = h.submit()
  await h.submit(); await pending
  assert.equal(calls, 1)
  assert.equal(h.storage.get(h.key), 'new-task')
  assert.equal(h.busy.value, true)
  await h.poll()
  assert.equal(h.busy.value, false)
  assert.equal(h.timers.size, 0)
  assert.equal(h.fullVideoUrl.value, 'https://app.test/ocopion/api/txt-video/files/example.mp4')
})

test('刷新恢复查询，网络中断保留任务，恢复后展示失败并允许重新提交', async () => {
  let online = false
  const h = harness({ getVideoTask: async () => {
    if (!online) throw new Error('offline')
    return { id: 'saved-task', status: 'failed', error: '合成失败' }
  } }, 'saved-task')
  h.mounted[0]()
  await new Promise((r) => setImmediate(r))
  assert.equal(h.task.value.id, 'saved-task')
  assert.ok(h.queryError.value)
  assert.equal(h.storage.get(h.key), 'saved-task')
  assert.equal(h.busy.value, true)
  online = true; await h.poll()
  assert.equal(h.task.value.error, '合成失败')
  assert.equal(h.queryError.value, '')
  assert.equal(h.busy.value, false)
})

test('任务不存在时清理存储；卸载释放预览与定时器，迟到响应不再修改状态', async () => {
  const h = harness({ getVideoTask: async () => { throw { data: { code: 404 } } } }, 'expired')
  h.mounted[0](); await new Promise((r) => setImmediate(r))
  assert.equal(h.task.value, undefined)
  assert.equal(h.storage.has(h.key), false)
  assert.ok(h.notice.value)
  h.selectCover({ target: { files: [new File(['x'], 'a.png', { type: 'image/png' })] } })
  h.unmounted[0]()
  assert.deepEqual(h.revoked, ['blob:preview'])
  assert.equal(h.timers.size, 0)
  let release
  const late = harness({ createVideoTask: () => new Promise((r) => { release = r }) })
  late.text.value = '文案'; late.cover.value = new File(['x'], 'a.png')
  const sending = late.submit(); late.unmounted[0]()
  release({ id: 'late-task', status: 'running' }); await sending
  assert.equal(late.storage.has(late.key), false)
  assert.equal(late.timers.size, 0)
})
