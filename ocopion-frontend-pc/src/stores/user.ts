import { computed, ref } from 'vue'
import { defineStore } from 'pinia'
import {
  login as loginApi,
  refreshAccessToken as refreshAccessTokenApi,
  type AuthTokenResponse,
} from '@/api/global'
import { getUserInfo, type UserInfo } from '@/api/user'

const ACCESS_TOKEN_KEY = 'accessToken'
const REFRESH_TOKEN_KEY = 'refreshToken'

export const useUserStore = defineStore('user', () => {
  const sessionVersion = ref(0)
  const accessToken = ref(localStorage.getItem(ACCESS_TOKEN_KEY) || '')
  const refreshToken = ref(localStorage.getItem(REFRESH_TOKEN_KEY) || '')
  const userInfo = ref<UserInfo | null>(null)
  const userInfoLoading = ref(false)
  const displayName = computed(() => userInfo.value?.nickname || userInfo.value?.username || '用户')
  let userInfoPromise: Promise<UserInfo> | null = null

  function setTokens(tokens: AuthTokenResponse) {
    accessToken.value = tokens.accessToken
    refreshToken.value = tokens.refreshToken
    localStorage.setItem(ACCESS_TOKEN_KEY, tokens.accessToken)
    localStorage.setItem(REFRESH_TOKEN_KEY, tokens.refreshToken)
  }

  async function refreshAccessToken() {
    if (!refreshToken.value) {
      return Promise.reject(new Error('Refresh token missing'))
    }

    const session = sessionVersion.value
    const response = await refreshAccessTokenApi(refreshToken.value)
    if (session !== sessionVersion.value) throw new Error('登录会话已变更')
    setTokens(response)
    return response.accessToken
  }

  function fetchUserInfo(force = false) {
    if (userInfo.value && !force) {
      return Promise.resolve(userInfo.value)
    }
    if (userInfoPromise) {
      return userInfoPromise
    }

    const session = sessionVersion.value
    userInfoLoading.value = true
    const promise = getUserInfo()
      .then((info) => {
        if (session !== sessionVersion.value) throw new Error('登录会话已变更')
        userInfo.value = info
        return info
      })
      .finally(() => {
        if (userInfoPromise === promise) {
          userInfoLoading.value = false
          userInfoPromise = null
        }
      })

    userInfoPromise = promise
    return promise
  }

  async function loginByEmail(email: string, password: string) {
    logout()
    const session = sessionVersion.value
    try {
      const tokens = await loginApi({ email, password })
      if (session !== sessionVersion.value) throw new Error('登录会话已变更')
      setTokens(tokens)
      return await fetchUserInfo()
    } catch (error) {
      if (session === sessionVersion.value) logout()
      throw error
    }
  }

  function logout() {
    sessionVersion.value += 1
    userInfoPromise = null
    userInfoLoading.value = false
    accessToken.value = ''
    refreshToken.value = ''
    userInfo.value = null
    localStorage.removeItem(ACCESS_TOKEN_KEY)
    localStorage.removeItem(REFRESH_TOKEN_KEY)
  }

  return {
    sessionVersion,
    loginByEmail,
    accessToken,
    refreshToken,
    userInfo,
    userInfoLoading,
    displayName,
    setTokens,
    refreshAccessToken,
    fetchUserInfo,
    logout,
  }
})
