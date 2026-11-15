import { API_BASE_URL, get, post } from '@/utils/request'
import { useUserStore } from '@/stores/user'
export interface NodeInfo {
  nodeName: string
  clientIP: string
  addressFamily: string
}
export const getSpeedtestInfo = (signal: AbortSignal) =>
  get<NodeInfo>('/info', undefined, { signal, timeout: 5000, silent: true })
export const download = (signal: AbortSignal, onChunk: (chunk: Uint8Array) => void) =>
  get<null>('/download', undefined, { signal, onChunk, silent: true })
export const upload = (data: Blob, signal: AbortSignal) =>
  post<{ bytes: number }>('/upload', data, {
    signal,
    // 后端允许读取 10 秒，额外预留代理转发与响应时间。
    timeout: 15000,
    retryAuth: false,
    silent: true,
    headers: { 'Content-Type': 'application/octet-stream' },
  })
export function openEcho() {
  const url = new URL(`${API_BASE_URL}/echo`, window.location.origin)
  url.protocol = url.protocol === 'https:' ? 'wss:' : 'ws:'
  return new WebSocket(url, ['ocopion-speedtest', `access_token.${useUserStore().accessToken}`])
}
