import { get } from '@/utils/request'

export interface UserInfo {
  id: number
  email: string
  phone: string
  avatarUrl: string
  status: string
  createdAt: string
  updatedAt: string
  username: string
  nickname: string
}

export function getUserInfo() {
  return get<UserInfo>('/user/me')
}
