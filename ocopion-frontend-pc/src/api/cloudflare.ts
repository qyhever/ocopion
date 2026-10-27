import { post } from '@/utils/request'

export interface SpeedTestResult {
  ips: string[]
}

export function speedTest() {
  return post<SpeedTestResult>('/cloudflare/speed-test', undefined, { timeout: 610_000 })
}
