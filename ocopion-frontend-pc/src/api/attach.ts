import { post } from '@/utils/request'

export interface AttachUploadResponse {
  fileName: string
  originName: string
  url: string
}

export function uploadAttach(file: File, onUploadProgress: (event: ProgressEvent) => void) {
  const data = new FormData()
  data.append('file', file)
  return post<AttachUploadResponse>('/attach/upload', data, { onUploadProgress })
}
