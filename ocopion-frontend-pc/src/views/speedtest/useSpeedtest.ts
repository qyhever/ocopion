import { computed, onBeforeUnmount, onDeactivated, reactive, ref, watch } from 'vue'
import { useUserStore } from '@/stores/user'
import * as api from '@/api/speedtest'
import { createRunner, initialState, statistics } from './engine'
export const phaseLabels = {
  ready: '准备就绪',
  connecting: '连接与预热',
  idle: '空闲延迟采样',
  download: '下载测试',
  upload: '上传测试',
  complete: '测试完成',
  stopped: '已停止 · 未完成',
  error: '测试异常 · 未完成',
}
const number = (value: number | null) => (value === null ? '样本不足' : value.toFixed(2))
export function useSpeedtest() {
  const state = reactive(initialState())
  const info = ref<api.NodeInfo | null>(null)
  const running = ref(false)
  let runner: ReturnType<typeof createRunner> | null = null
  const stop = () => runner?.stop()
  async function start() {
    if (running.value) return
    Object.assign(state, initialState())
    info.value = null
    running.value = true
    runner = createRunner(
      state,
      {
        info: api.getSpeedtestInfo,
        echo: api.openEcho,
        download: api.download,
        upload: api.upload,
      },
      (value) => {
        info.value = value
      },
    )
    try {
      await runner.run()
    } finally {
      runner = null
      running.value = false
    }
  }
  const resultText = computed(() => {
    const lines = [
      'VPS 测速',
      `状态：${phaseLabels[state.phase]}`,
      `节点：${info.value?.nodeName ?? '—'}`,
      `客户端出口 IP：${info.value?.clientIP ?? '—'} (${info.value?.addressFamily ?? '—'})`,
      `握手：${number(state.handshake)} ms`,
    ]
    for (const [name, result] of [
      ['下载', state.download],
      ['上传', state.upload],
    ] as const)
      lines.push(
        `${name}：${result.rate.toFixed(2)} Mbps，${(result.elapsed / 1000).toFixed(2)} 秒，${result.bytes} 字节，${result.complete ? '完成' : '未完成'}`,
      )
    for (const [name, value] of [
      ['空闲', state.idle],
      ['下载负载', state.downloadLatency],
      ['上传负载', state.uploadLatency],
    ] as const) {
      const s = statistics(value)
      lines.push(
        `${name}：中位数 ${number(s.median)} ms / P95 ${number(s.p95)} ms / 抖动 ${number(s.jitter)} ms / 请求失败率 ${number(s.failure)}% (${value.failed}/${value.sent})`,
      )
    }
    lines.push('请求失败率不是 IP 层丢包率。速率采用十进制 Mbps。')
    if (state.error) lines.push(state.error)
    return lines.join('\n')
  })
  async function copy() {
    await navigator.clipboard.writeText(resultText.value)
  }
  watch(() => useUserStore().sessionVersion, stop)
  onDeactivated(stop)
  onBeforeUnmount(stop)
  return { state, info, running, start, stop, resultText, copy }
}
