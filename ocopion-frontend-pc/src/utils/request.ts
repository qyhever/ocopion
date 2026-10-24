/* eslint-disable @typescript-eslint/no-explicit-any */
import { MessagePlugin } from 'tdesign-vue-next'
import { useUserStore } from '@/stores/user'
import router from '@/router'
import { request as http } from './fetch'
import type { RequestOptions } from './fetch'

const defaultOptions = { baseURL: '/ocopion/api' }
const SUCCESS_CODE = 200
let refreshTask: { session: number; promise: Promise<string> } | null = null
let expiredSession: number | null = null

const codeMessage: Record<number, string> = {
  400: '请求错误',
  401: '登录状态失效，请重新登录',
  403: '禁止访问',
  404: '请求地址不存在',
  500: '服务器繁忙，请稍后再试',
  502: '网关错误',
  503: '服务不可用，服务器暂时过载或维护',
  504: '网关超时',
}

interface RequestError extends Error {
  response?: { status: number }
  data?: { code: number; message: string }
}

function expireSession(session: number) {
  const userStore = useUserStore()
  if (expiredSession === session || userStore.sessionVersion !== session) return
  expiredSession = session
  userStore.logout()
  MessagePlugin.error('登录已过期，请重新登录')
  const route = router.currentRoute.value
  if (route.path !== '/signin') {
    void router.replace({ path: '/signin', query: { redirect: route.fullPath } })
  }
}

function refreshToken(session: number) {
  if (!refreshTask || refreshTask.session !== session) {
    const task = {
      session,
      promise: useUserStore()
        .refreshAccessToken()
        .finally(() => {
          if (refreshTask === task) refreshTask = null
        }),
    }
    refreshTask = task
  }
  return refreshTask.promise
}

export async function request<T = any>(opts: RequestOptions): Promise<T> {
  const options = { ...defaultOptions, ...opts }
  const isAuthRequest = ['/auth/login', '/auth/refresh'].includes(options.url)
  const isRefreshRequest = options.url === '/auth/refresh'
  const userStore = useUserStore()
  const session = userStore.sessionVersion

  async function send(retried = false): Promise<T> {
    const token = userStore.accessToken
    let response: IApiResponse<T>
    try {
      response = await http<IApiResponse<T>>({
        ...options,
        headers: {
          ...options.headers,
          ...(!isAuthRequest && token ? { Authorization: `Bearer ${token}` } : {}),
        },
        params: { ...options.params, t: Date.now() },
      })
    } catch (cause) {
      const error = cause as RequestError
      if (!isAuthRequest && error.response?.status === 401) {
        // 旧会话的请求不能清理或刷新新会话。
        if (userStore.sessionVersion !== session) throw error
        if (retried) {
          expireSession(session)
          throw error
        }
        try {
          // 较晚返回的旧 Token 401 复用已经刷新的 Token。
          if (token === userStore.accessToken) await refreshToken(session)
        } catch (refreshError) {
          expireSession(session)
          throw refreshError
        }
        if (userStore.sessionVersion !== session) throw error
        return send(true)
      }
      // 刷新失败由原始受保护请求统一处理，避免并发重复提示。
      if (!isRefreshRequest) {
        MessagePlugin.error(
          error.data?.message ||
            (error.response ? codeMessage[error.response.status] : error.message) ||
            '网络异常，请稍后重试',
        )
      }
      throw cause
    }

    if (response.code !== SUCCESS_CODE) {
      const error: RequestError = new Error(response.message || '操作失败')
      error.data = response
      if (!isRefreshRequest) MessagePlugin.warning(error.message)
      throw error
    }
    return response.data
  }

  return send()
}

export async function get<T = any>(
  url: string,
  params?: Record<string, any>,
  options?: Omit<RequestOptions, 'url' | 'params' | 'method'>,
) {
  return request<T>({
    method: 'GET',
    url,
    params,
    ...options,
  })
}

export async function post<T = any>(
  url: string,
  data?: any,
  options?: Omit<RequestOptions, 'url' | 'data' | 'method'>,
) {
  return request<T>({
    method: 'POST',
    url,
    data,
    ...options,
  })
}

export async function put<T = any>(
  url: string,
  data?: any,
  options?: Omit<RequestOptions, 'url' | 'data' | 'method'>,
) {
  return request<T>({
    method: 'PUT',
    url,
    data,
    ...options,
  })
}

export async function del<T = any>(
  url: string,
  params?: Record<string, any>,
  options?: Omit<RequestOptions, 'url' | 'params' | 'method'>,
) {
  return request<T>({
    method: 'DELETE',
    url,
    params,
    ...options,
  })
}

export async function patch<T = any>(
  url: string,
  data?: any,
  options?: Omit<RequestOptions, 'url' | 'data' | 'method'>,
) {
  return request<T>({
    method: 'PATCH',
    url,
    data,
    ...options,
  })
}
