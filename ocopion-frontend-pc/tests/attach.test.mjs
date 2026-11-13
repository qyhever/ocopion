import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import vm from 'node:vm'
import { test } from 'node:test'
import ts from 'typescript'

function load(relative, imports, extra = {}) {
  const source = readFileSync(new URL(relative, import.meta.url), 'utf8')
    .replaceAll('import.meta.url', "'http://localhost/'")
  const { outputText } = ts.transpileModule(source, {
    compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2022 },
  })
  const module = { exports: {} }
  vm.runInNewContext(outputText, { module, exports: module.exports, require: (s) => imports[s],
    URL, Blob, FormData, Error, console, ...extra })
  return module.exports
}
function harness(overrides = {}, options = {}) {
  const calls = [], progress = [], errors = [], successes = []
  const api = {
    uploadVerify: async (p) => { calls.push(['verify', p]); return { isExists: false, uploadId: 'task', uploadedChunks: [] } },
    uploadChunk: async (p) => { calls.push(['chunk', p.chunkIndex]) },
    mergeChunks: async (p) => { calls.push(['merge', p]); return { url: '/done.mp4', msg: 'ok' } },
    ...overrides,
  }
  class Worker {
    postMessage() { queueMicrotask(() => this.onmessage({ data: { type: 'success', md5: 'fake-md5' } })) }
    terminate() {}
  }
  const { default: Uploader } = load('../src/utils/chunk-upload.ts', { '@/api/chunk': api }, { Worker })
  const uploader = new Uploader({ chunkSize: 2, onProgress: (p) => progress.push(p),
    onError: (e) => errors.push(e), onSuccess: (r) => successes.push(r), ...options })
  const file = new File(['abcdefghij'], 'video.MP4')
  return { uploader, file, calls, progress, errors, successes }
}
test('分片完整流程传实际 chunkSize，并合并全部分片', async () => {
  const h = harness()
  await h.uploader.start(h.file)
  assert.equal(h.calls[0][1].chunkSize, 2)
  assert.equal(h.calls.filter((c) => c[0] === 'chunk').length, 5)
  assert.equal(h.calls.at(-1)[0], 'merge')
  assert.equal(h.progress.at(-1), 100)
  assert.equal(h.successes.length, 1)
  assert.equal(h.errors.length, 0)
})
test('续传跳过已有分片，秒传直接成功', async () => {
  const h = harness({ uploadVerify: async () => ({ uploadId: 'task', uploadedChunks: [0, 2, 4] }) })
  await h.uploader.start(h.file)
  assert.deepEqual(h.calls.filter((c) => c[0] === 'chunk').map((c) => c[1]), [1, 3])
  assert.equal(h.successes.length, 1)
  const instant = harness({ uploadVerify: async () => ({ isExists: true, url: '/exists.mp4' }) })
  await instant.uploader.start(instant.file)
  assert.equal(instant.calls.length, 0)
  assert.equal(instant.successes[0].url, '/exists.mp4')
})
test('重试耗尽停止派发，等待在途请求完成后报告失败，迟到回调不更新进度', async () => {
  let release, attempts = 0
  const pending = new Promise((r) => { release = r })
  const sent = []
  const h = harness({ uploadChunk: async (p) => {
    sent.push(p.chunkIndex)
    if (p.chunkIndex === 0) { attempts++; throw new Error('failed') }
    await pending
  } }, { concurrency: 2 })
  const running = h.uploader.start(h.file)
  await new Promise((r) => setImmediate(r))
  assert.equal(attempts, 3)
  assert.equal(h.errors.length, 0)
  assert.deepEqual(sent, [0, 1, 0, 0])
  release()
  await running
  assert.equal(h.errors.length, 1)
  assert.equal(h.progress.length, 0)
  assert.equal(h.successes.length, 0)
  assert.equal(h.calls.some((c) => c[0] === 'merge'), false)
})
test('常规上传 API 使用 file 字段并透传真实进度回调', async () => {
  let captured
  const { uploadAttach } = load('../src/api/attach.ts', { '@/utils/request': { post: async (...args) => { captured = args; return { url: '/x' } } } })
  const progress = () => {}
  await uploadAttach(new File(['abc'], 'a.txt'), progress)
  assert.equal(captured[0], '/attach/upload')
  assert.equal(captured[1].get('file').name, 'a.txt')
  assert.equal(captured[2].onUploadProgress, progress)
})
test('空文件在计算指纹前失败', async () => {
  const h = harness()
  await h.uploader.start(new File([], 'empty.mp4'))
  assert.equal(h.errors.length, 1)
  assert.equal(h.calls.length, 0)
})
