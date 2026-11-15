export type Phase =
  | 'ready'
  | 'connecting'
  | 'idle'
  | 'download'
  | 'upload'
  | 'complete'
  | 'stopped'
  | 'error'
export interface Samples {
  sent: number
  failed: number
  rtts: number[]
}
export const samples = (): Samples => ({ sent: 0, failed: 0, rtts: [] })
export const mbps = (bytes: number, milliseconds: number) =>
  milliseconds > 0 ? (bytes * 8) / milliseconds / 1000 : 0
export function statistics(s: Samples) {
  const sorted = [...s.rtts].sort((a, b) => a - b)
  const n = sorted.length
  return {
    median: n ? (sorted[Math.floor((n - 1) / 2)]! + sorted[Math.floor(n / 2)]!) / 2 : null,
    p95: n >= 2 ? sorted[Math.ceil(n * 0.95) - 1]! : null,
    jitter:
      n >= 2
        ? s.rtts.slice(1).reduce((sum, value, i) => sum + Math.abs(value - s.rtts[i]!), 0) / (n - 1)
        : null,
    failure: s.sent ? (s.failed / s.sent) * 100 : null,
  }
}
export interface Transfer {
  bytes: number
  elapsed: number
  rate: number
  complete: boolean
}
export const transfer = (): Transfer => ({ bytes: 0, elapsed: 0, rate: 0, complete: false })
export interface State {
  phase: Phase
  handshake: number | null
  liveRate: number
  error: string
  idle: Samples
  downloadLatency: Samples
  uploadLatency: Samples
  download: Transfer
  upload: Transfer
}
export const initialState = (): State => ({
  phase: 'ready',
  handshake: null,
  liveRate: 0,
  error: '',
  idle: samples(),
  downloadLatency: samples(),
  uploadLatency: samples(),
  download: transfer(),
  upload: transfer(),
})
export interface Transport {
  info(signal: AbortSignal): Promise<{ nodeName: string; clientIP: string; addressFamily: string }>
  echo(): WebSocket
  download(signal: AbortSignal, chunk: (data: Uint8Array) => void): Promise<unknown>
  upload(data: Blob, signal: AbortSignal): Promise<{ bytes: number }>
}
// 资源都归属于一次运行；停止会同步取消请求、采样和所有等待中的回显。
export function createRunner(
  state: State,
  transport: Transport,
  onInfo: (info: Awaited<ReturnType<Transport['info']>>) => void,
  now = () => performance.now(),
  duration = 10000,
) {
  const controller = new AbortController()
  const signal = controller.signal
  let socket: WebSocket | undefined
  let connectionError: Error | null = null
  let sequence = 0
  const pending = new Map<number, { finish: (success: boolean) => void }>()
  const timers = new Map<ReturnType<typeof setTimeout>, () => void>()
  const aborted = () => {
    if (signal.aborted) throw new Error('测试已停止')
  }
  function delay(ms: number) {
    return new Promise<void>((resolve) => {
      const finish = () => {
        clearTimeout(id)
        timers.delete(id)
        resolve()
      }
      const id = setTimeout(finish, ms)
      timers.set(id, finish)
      if (signal.aborted) finish()
    })
  }
  function stop() {
    controller.abort()
    if (socket) {
      socket.onopen = null
      socket.onmessage = null
      socket.onerror = null
      socket.onclose = null
      socket.close()
    }
    for (const p of pending.values()) p.finish(false)
    for (const finish of timers.values()) finish()
  }
  async function probe(target: Samples) {
    aborted()
    target.sent++
    const id = ++sequence
    const start = now()
    await new Promise<void>((resolve) => {
      const finish = (success: boolean) => {
        if (!pending.delete(id)) return
        clearTimeout(timer)
        if (success) target.rtts.push(now() - start)
        else target.failed++
        resolve()
      }
      const timer = setTimeout(() => finish(false), 2000)
      pending.set(id, { finish })
      try {
        socket!.send(String(id))
      } catch {
        finish(false)
      }
    })
  }
  async function loadSamples(target: Samples, active: () => boolean) {
    const tasks: Promise<void>[] = []
    while (active() && !signal.aborted) {
      tasks.push(probe(target))
      await delay(500)
    }
    await Promise.all(tasks)
  }
  async function measure(kind: 'download' | 'upload') {
    state.phase = kind
    state.liveRate = 0
    const result = state[kind]
    const start = now()
    const update = () => {
      result.elapsed = now() - start
      result.rate = mbps(result.bytes, result.elapsed)
      state.liveRate = result.rate
    }
    let active = true
    const sampling = loadSamples(
      kind === 'download' ? state.downloadLatency : state.uploadLatency,
      () => active,
    )
    try {
      if (kind === 'download') {
        const downloadController = new AbortController()
        const abort = () => downloadController.abort()
        signal.addEventListener('abort', abort, { once: true })
        let expired = false
        const timer = setTimeout(() => {
          expired = true
          downloadController.abort()
        }, duration)
        try {
          await transport.download(downloadController.signal, (data) => {
            result.bytes += data.byteLength
            update()
          })
          if (!expired) throw new Error('下载流提前结束')
        } catch (error) {
          if (!expired || signal.aborted) throw error
        } finally {
          clearTimeout(timer)
          signal.removeEventListener('abort', abort)
          downloadController.abort()
          update()
        }
        if (!result.bytes) throw new Error('未接收到下载数据')
      } else {
        let size = 64 * 1024
        const payload = new Uint8Array(8 * 1024 * 1024)
        // 每次最多填充 64 KiB，兼容浏览器 getRandomValues 上限。
        for (let i = 0; i < payload.length; i += 65536)
          crypto.getRandomValues(payload.subarray(i, i + 65536))
        while (now() - start < duration) {
          aborted()
          const sent = now()
          const confirmed = await transport.upload(new Blob([payload.subarray(0, size)]), signal)
          if (!Number.isInteger(confirmed.bytes) || confirmed.bytes !== size)
            throw new Error('服务端上传确认字节数不匹配')
          result.bytes += confirmed.bytes
          update()
          size = Math.min(
            8 * 1024 * 1024,
            // 限制扩容，避免短时快速响应导致下一块过大；慢速时允许缩至 4 KiB。
            size * 2,
            Math.max(4096, Math.round((size * 500) / Math.max(1, now() - sent))),
          )
        }
      }
      aborted()
      result.complete = true
    } finally {
      update()
      active = false
      await sampling
    }
  }
  async function run() {
    try {
      state.phase = 'connecting'
      onInfo(await transport.info(signal))
      aborted()
      const start = now()
      socket = transport.echo()
      await new Promise<void>((resolve, reject) => {
        const timer = setTimeout(() => {
          cleanup()
          socket?.close()
          reject(new Error('回显连接超时'))
        }, 5000)
        const cleanup = () => {
          clearTimeout(timer)
          signal.removeEventListener('abort', abort)
        }
        const abort = () => {
          cleanup()
          reject(new Error('测试已停止'))
        }
        signal.addEventListener('abort', abort, { once: true })
        socket!.onopen = () => {
          cleanup()
          resolve()
        }
        socket!.onerror = () => {
          cleanup()
          reject(new Error('回显连接失败'))
        }
        socket!.onclose = () => {
          cleanup()
          reject(new Error('回显连接已关闭'))
        }
      })
      state.handshake = now() - start
      socket.onmessage = (event) => {
        const id = Number(event.data)
        pending.get(id)?.finish(true)
      }
      const connectionFailed = () => {
        connectionError = new Error('回显连接中断')
        stop()
      }
      socket.onclose = connectionFailed
      socket.onerror = connectionFailed
      await probe(samples())
      aborted()
      state.phase = 'idle'
      const tasks: Promise<void>[] = []
      for (let i = 0; i < 20; i++) {
        aborted()
        tasks.push(probe(state.idle))
        await delay(200)
      }
      await Promise.all(tasks)
      if (!state.idle.rtts.length) throw new Error('回显采样全部失败')
      aborted()
      await measure('download')
      await measure('upload')
      state.phase = 'complete'
    } catch (error) {
      state.phase = signal.aborted && !connectionError ? 'stopped' : 'error'
      if (connectionError) state.error = connectionError.message
      else if (!signal.aborted) state.error = error instanceof Error ? error.message : '测速失败'
    } finally {
      stop()
      state.liveRate = 0
    }
  }
  return { run, stop }
}
