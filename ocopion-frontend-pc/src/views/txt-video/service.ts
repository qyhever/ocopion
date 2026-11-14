import { get, post } from '@/utils/request'

export interface VideoTask {
  id: string
  status: 'running' | 'succeeded' | 'failed'
  error: string
  videoUrl?: string
  downloadUrl?: string
}

export function createVideoTask(text: string, cover: File, speed: string) {
  const data = new FormData()
  data.append('text', text)
  data.append('cover', cover)
  data.append('speed', speed)
  return post<VideoTask>('/txt-video/tasks', data)
}

export function getVideoTask(id: string) {
  return get<VideoTask>(`/txt-video/tasks/${encodeURIComponent(id)}`)
}
