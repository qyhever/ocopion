import assert from 'node:assert/strict'
import { test } from 'node:test'
import { readFileSync } from 'node:fs'
import vm from 'node:vm'
import ts from 'typescript'
import { webcrypto } from 'node:crypto'
function harness({ late = false, uploadFails = false, uploadDelay = 100 } = {}) {
  let time = 0, next = 0, activeUploads = 0, maxUploads = 0, closed = false
  const timers = new Map(), phases = [], uploadSizes = []
  const set = (fn, ms) => { const id = ++next; timers.set(id, { fn, at: time + ms }); return id }
  const clear = id => timers.delete(id)
  const module = { exports: {} }
  const context = vm.createContext({ module, exports: module.exports, AbortController, Blob, Uint8Array, crypto: webcrypto, performance: { now: () => time }, setTimeout: set, clearTimeout: clear })
  vm.runInContext(ts.transpileModule(readFileSync(new URL('../src/views/speedtest/engine.ts', import.meta.url), 'utf8'), { compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2022 } }).outputText, context)
  const engine = module.exports
  const state = new Proxy(engine.initialState(), { set(target,key,value) { if (key === 'phase') phases.push(value); target[key] = value; return true } })
  let socket
  const transport = {
    async info() { return { nodeName: 'test', clientIP: '::1', addressFamily: 'IPv6' } },
    echo() {
      socket = { send(data) { set(() => socket.onmessage?.({ data }), late && data === '2' ? 2100 : 10) }, close() { closed = true; socket.onclose?.() } }
      set(() => socket.onopen(), 10)
      return socket
    },
    download(signal, chunk) {
      return new Promise((resolve,reject) => {
        const send = () => { chunk(new Uint8Array(10000)); timer = set(send,100) }
        let timer = set(send,100)
        signal.addEventListener('abort', () => { clear(timer); reject(new Error('cancelled')) }, { once: true })
      })
    },
    upload(blob, signal) {
      uploadSizes.push(blob.size)
      activeUploads++; maxUploads = Math.max(maxUploads,activeUploads)
      return new Promise((resolve,reject) => {
        const done = () => { activeUploads--; signal.removeEventListener('abort', abort); uploadFails ? reject(new Error('upload failed')) : resolve({ bytes: blob.size }) }
        const timer = set(done,uploadDelay)
        const abort = () => { clear(timer); activeUploads--; reject(new Error('cancelled')) }
        signal.addEventListener('abort', abort, { once: true })
      })
    },
  }
  const runner = engine.createRunner(state,transport,()=>{},()=>time,1000)
  async function advance(ms) {
    const target = time+ms
    for (let steps=0;steps<100000;steps++) {
      for (let i=0;i<20;i++) await Promise.resolve()
      const entry = [...timers.entries()].filter(([,v]) => v.at<=target).sort((a,b)=>a[1].at-b[1].at)[0]
      if (!entry) { time=target; for (let i=0;i<20;i++) await Promise.resolve(); return }
      time=entry[1].at; timers.delete(entry[0]); entry[1].fn()
    }
    throw new Error('timer loop')
  }
  return { engine,state,runner,advance,phases,timers,uploadSizes,disconnect(){socket.onclose?.()},get maxUploads(){return maxUploads},get closed(){return closed} }
}
test('统计公式与十进制 Mbps', () => {
  const { engine }=harness()
  const v=engine.statistics({ sent:5,failed:1,rtts:[10,30,20,40] })
  assert.equal(v.median,25); assert.equal(v.p95,40); assert.equal(v.jitter,50/3); assert.equal(v.failure,20)
  assert.equal(engine.mbps(1250000,1000),10)
  assert.equal(engine.statistics(engine.samples()).median,null)
  assert.equal(engine.statistics({sent:1,failed:0,rtts:[10]}).jitter,null)
})
test('阶段顺序、20 次空闲采样、单个在途上传与服务端确认计速', async () => {
  const h=harness();const p=h.runner.run();await h.advance(10000);await p
  assert.deepEqual(h.phases,['connecting','idle','download','upload','complete'])
  assert.equal(h.state.idle.sent,20);assert.equal(h.state.idle.rtts.length,20)
  assert.equal(h.maxUploads,1);assert.equal(h.state.upload.complete,true)
  assert.equal(h.state.upload.rate,h.engine.mbps(h.state.upload.bytes,h.state.upload.elapsed))
  assert.equal(h.closed,true);assert.equal(h.timers.size,0)
})
test('超时与迟到回显', async () => {
  const h=harness({late:true});const p=h.runner.run();await h.advance(10000);await p
  assert.equal(h.state.idle.sent,20);assert.equal(h.state.idle.failed,1);assert.equal(h.state.idle.rtts.length,19)
})
test('停止清理并保留未完成结果', async () => {
  const h=harness();const p=h.runner.run();await h.advance(4400)
  assert.equal(h.state.phase,'download');h.runner.stop();await h.advance(100);await p
  assert.equal(h.state.phase,'stopped');assert.equal(h.state.download.complete,false);assert.equal(h.closed,true);assert.equal(h.timers.size,0)
})
test('上传失败不累计未确认流量', async () => {
  const h=harness({uploadFails:true});const p=h.runner.run();await h.advance(10000);await p
  assert.equal(h.state.phase,'error');assert.equal(h.state.upload.bytes,0);assert.equal(h.state.upload.complete,false)
})

// 实际请求封装回归：使用统一认证，二进制不做 JSON 序列化，流不自动重传。
const { createHarness } = await import('./auth.test.mjs')
const ok = data => new Response(JSON.stringify({ code:200, data }), { headers: { 'Content-Type':'application/json' } })
test('二进制上传与响应确认', async () => {
  const body = new Blob([new Uint8Array([1,2,3])])
  const h = createHarness(call => { assert.equal(call.body, body); return ok({ bytes:3 }) })
  const response = await h.api.post('/upload', body, { retryAuth:false })
  assert.equal(response.bytes,3);assert.equal(h.calls.length,1)
})
test('逐块下载，流开始后出错不会重试', async () => {
  let reads=0, total=0
  const h=createHarness(() => new Response(new ReadableStream({ pull(controller) { if (++reads===1) controller.enqueue(new Uint8Array([1,2,3])); else controller.error(new Error('stream failed')) } })))
  await assert.rejects(h.api.get('/download',undefined,{ onChunk:data => { total+=data.length },silent:true }), /stream failed/)
  assert.equal(total,3);assert.equal(h.calls.length,1)
})
test('上传 401 刷新凭据但不重传上传体', async () => {
  const h=createHarness(call => call.url.pathname.endsWith('/auth/refresh') ? ok({ accessToken:'fresh',refreshToken:'refresh' }) : new Response('{}',{status:401,headers:{'Content-Type':'application/json'}}))
  await assert.rejects(h.api.post('/upload',new Blob(['data']),{ retryAuth:false,silent:true }))
  assert.equal(h.calls.filter(c => c.url.pathname.endsWith('/upload')).length,1)
  assert.equal(h.store.accessToken,'fresh')
})

test('组合函数防重复启动、重测清空旧结果、停止/离页/退出登录清理与复制', async () => {
  let creates=0, stops=0, finish, watched, clipboard=''
  const disposers=[]
  const module={exports:{}}
  const context=vm.createContext({module,exports:module.exports,navigator:{clipboard:{async writeText(text){clipboard=text}}},require(name){
    if(name==='vue')return {reactive:x=>x,ref:value=>({value}),computed:fn=>({get value(){return fn()}}),watch:(_source,fn)=>{watched=fn},onDeactivated:fn=>disposers.push(fn),onBeforeUnmount:fn=>disposers.push(fn)}
    if(name==='@/stores/user')return {useUserStore:()=>({sessionVersion:1})}
    if(name==='@/api/speedtest')return {}
    if(name==='./engine')return {...harness().engine,createRunner(state){creates++;return {run(){return new Promise(resolve=>{finish=()=>{state.phase='stopped';resolve()}})},stop(){stops++;finish()}}}}
    throw new Error(name)
  }})
  const source=readFileSync(new URL('../src/views/speedtest/useSpeedtest.ts',import.meta.url),'utf8')
  vm.runInContext(ts.transpileModule(source,{compilerOptions:{module:ts.ModuleKind.CommonJS,target:ts.ScriptTarget.ES2022}}).outputText,context)
  const c=module.exports.useSpeedtest()
  const first=c.start();await c.start();assert.equal(creates,1)
  c.state.download.bytes=1000;c.stop();await first
  await c.copy();assert.match(clipboard,/未完成/);assert.match(clipboard,/1000 字节/)
  const second=c.start();assert.equal(c.state.download.bytes,0);assert.equal(creates,2)
  watched();await second
  const third=c.start();disposers[0]();await third
  const fourth=c.start();disposers[1]();await fourth
  assert.equal(stops,4);assert.equal(c.running.value,false)
})

test('回显连接断开立即取消流并标记异常', async () => {
  const h=harness();const p=h.runner.run();await h.advance(4400)
  h.disconnect();await h.advance(100);await p
  assert.equal(h.state.phase,'error');assert.match(h.state.error,/连接中断/)
  assert.equal(h.state.download.complete,false);assert.equal(h.timers.size,0)
})
test('上传期间停止不累计在途请求，保留下载完成结果', async () => {
  const h=harness();const p=h.runner.run();await h.advance(5600)
  assert.equal(h.state.phase,'upload');const confirmed=h.state.upload.bytes
  h.runner.stop();await h.advance(100);await p
  assert.equal(h.state.phase,'stopped');assert.equal(h.state.upload.bytes,confirmed)
  assert.equal(h.state.download.complete,true);assert.equal(h.state.upload.complete,false)
  assert.equal(h.timers.size,0)
})


test('上传块每次最多增长一倍', async () => {
  const h=harness();const p=h.runner.run();await h.advance(10000);await p
  assert.ok(h.uploadSizes.length > 2)
  for (let i=1;i<h.uploadSizes.length;i++) assert.ok(h.uploadSizes[i] <= h.uploadSizes[i-1]*2)
})
test('慢速上传超过旧的五秒限制仍按完整确认计速', async () => {
  const h=harness({uploadDelay:6000});const p=h.runner.run();await h.advance(13000);await p
  assert.equal(h.state.phase,'complete');assert.equal(h.state.upload.bytes,65536)
  assert.equal(h.state.upload.elapsed,6000);assert.equal(h.timers.size,0)
})
test('慢速上行允许上传块缩至 64 KiB 以下', async () => {
  const h=harness({uploadDelay:700});const p=h.runner.run();await h.advance(10000);await p
  assert.ok(h.uploadSizes[1] < 65536);assert.ok(h.uploadSizes[1] >= 4096)
  assert.equal(h.state.phase,'complete')
})
test('上传请求为后端读取与响应预留时间，并传递取消信号', async () => {
  let options
  const module={exports:{}}
  const body=new Blob(['data']), signal=new AbortController().signal
  const context=vm.createContext({module,exports:module.exports,require(name){
    if(name==='@/utils/request')return {post:async (url,data,opts)=>{assert.equal(url,'/upload');assert.equal(data,body);options=opts;return {bytes:4}}}
    if(name==='@/stores/user')return {}
    throw new Error(name)
  }})
  vm.runInContext(ts.transpileModule(readFileSync(new URL('../src/api/speedtest.ts',import.meta.url),'utf8'),{compilerOptions:{module:ts.ModuleKind.CommonJS,target:ts.ScriptTarget.ES2022}}).outputText,context)
  assert.equal((await module.exports.upload(body,signal)).bytes,4)
  assert.equal(options.timeout,15000);assert.equal(options.signal,signal);assert.equal(options.retryAuth,false)
})
