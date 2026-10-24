import { createRouter, createWebHistory } from 'vue-router'
import { useUserStore } from '@/stores/user'
import LoginView from '../views/LoginView.vue'
import BasicLayout from '../layouts/BasicLayout.vue'

const HOME_PATH = '/home'
const LOGIN_PATH = '/signin'

declare module 'vue-router' {
  interface RouteMeta {
    authKey?: string
    activeMenu?: string
  }
}

const basicRoutes = [
  {
    path: HOME_PATH,
    name: 'home',
    component: () => import('../views/HomeView.vue'),
  },
  {
    path: '/attach/simple-upload',
    name: 'simple-upload',
    component: () => import('../views/attach/SimpleUploadView.vue'),
    meta: {
      title: '常规上传',
    },
  },
  {
    path: '/attach/chunk-upload',
    name: 'chunk-upload',
    component: () => import('../views/attach/ChunkUploadView.vue'),
    meta: {
      title: '分片上传',
    },
  },
  {
    path: '/mail/send',
    name: 'mail-send',
    component: () => import('../views/mail/SendMail.vue'),
    meta: {
      title: '发送邮件',
    },
  },
  {
    path: '/notification/tg',
    name: 'telegram-send',
    component: () => import('../views/telegram/SendTelegram.vue'),
    meta: {
      title: '发送Telegram消息',
    },
  },
]

const router = createRouter({
  history: createWebHistory(import.meta.env.BASE_URL),
  routes: [
    {
      path: '/',
      redirect: HOME_PATH,
    },
    {
      path: LOGIN_PATH,
      name: 'signin',
      component: LoginView,
    },
    {
      path: '/',
      component: BasicLayout,
      children: basicRoutes,
    },
    {
      path: '/forbidden',
      component: () => import('@/views/result/ForbiddenView.vue'),
    },
    {
      path: '/notfound',
      component: () => import('@/views/result/NotfoundView.vue'),
    },
    {
      path: '/:pathMatch(.*)*',
      component: () => import('@/views/result/NotfoundView.vue'),
    },
    {
      path: '/about',
      name: 'about',
      // route level code-splitting
      // this generates a separate chunk (About.[hash].js) for this route
      // which is lazy-loaded when the route is visited.
      component: () => import('../views/AboutView.vue'),
    },
  ],
})

// 仅允许已注册的站内页面，避免外部地址、登录页和兜底 404。
export function getLoginRedirect(value: unknown): string {
  if (typeof value !== 'string' || !value.startsWith('/') || /^\/\/|[\\\s]/.test(value)) {
    return HOME_PATH
  }
  const target = router.resolve(value)
  if (
    !target.matched.length ||
    target.matched.some(
      (record) =>
        [LOGIN_PATH, '/notfound'].includes(record.path) || record.path.includes(':pathMatch'),
    )
  ) {
    return HOME_PATH
  }
  return target.fullPath
}

router.beforeEach(async (to) => {
  const userStore = useUserStore()
  const isLoginPage = to.name === 'signin' || to.path === LOGIN_PATH
  const loginTarget = {
    path: LOGIN_PATH,
    query: to.fullPath === HOME_PATH ? undefined : { redirect: to.fullPath },
  }

  if (!userStore.accessToken) return isLoginPage ? true : loginTarget

  if (!userStore.userInfo) {
    try {
      await userStore.fetchUserInfo()
    } catch {
      // 请求层在认证失败时清理会话；网络异常保留 Token 并中止导航。
      if (!userStore.accessToken) return isLoginPage ? true : loginTarget
      return false
    }
  }

  if (!userStore.accessToken) return isLoginPage ? true : loginTarget
  return isLoginPage ? getLoginRedirect(to.query.redirect) : true
})

export const getActive = (maxLevel = 3): string => {
  // 非组件内调用必须通过Router实例获取当前路由
  const route = router.currentRoute.value

  if (!route.path) {
    return ''
  }
  if (route.meta?.activeMenu) {
    return route.meta.activeMenu
  }
  const ret = route.path
    .split('/')
    .filter((_item: string, index: number) => index <= maxLevel && index > 0)
    .map((item: string) => `/${item}`)
    .join('')

  return ret
}

export default router
