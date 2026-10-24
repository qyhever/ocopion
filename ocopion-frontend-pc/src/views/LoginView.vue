<template>
  <div class="login-container">
    <div class="login-card">
      <div class="login-header">
        <h2>用户登录</h2>
        <p>欢迎回来，请输入您的邮箱和密码</p>
      </div>

      <t-form
        ref="formRef"
        :data="formData"
        :rules="rules"
        label-width="0"
        class="login-form"
        @submit="handleSubmit"
      >
        <t-form-item name="email">
          <t-input
            v-model="formData.email"
            placeholder="请输入邮箱"
            clearable
            size="large"
            autocomplete="username"
          >
            <template #prefix-icon>
              <t-icon name="mail" />
            </template>
          </t-input>
        </t-form-item>

        <t-form-item name="password">
          <t-input
            v-model="formData.password"
            type="password"
            autocomplete="current-password"
            placeholder="请输入密码"
            clearable
            size="large"
          >
            <template #prefix-icon>
              <t-icon name="lock-on" />
            </template>
          </t-input>
        </t-form-item>

        <t-form-item>
          <t-button type="submit" theme="primary" size="large" block :loading="loading">
            {{ loading ? '登录中...' : '登录' }}
          </t-button>
        </t-form-item>
      </t-form>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, onMounted, onBeforeUnmount } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { MessagePlugin, type FormRule } from 'tdesign-vue-next'
import { getLoginRedirect } from '@/router'
import { useUserStore } from '@/stores/user'
import type { FormProps } from 'tdesign-vue-next'
import { loadScript, removeScript } from '@/utils'

const canvasURL = import.meta.env.BASE_URL + 'assets/js/canvas-nest.js?v=' + LOCAL_BUILD_HASH
const router = useRouter()
const route = useRoute()
const userStore = useUserStore()
const formRef = ref()
const loading = ref(false)

// 表单数据
const formData = reactive({
  email: '',
  password: '',
})

// 表单验证规则
const rules: Record<string, FormRule[]> = {
  email: [
    { required: true, message: '请输入邮箱', type: 'error' as const },
    {
      pattern: /^[^\s@]+@[^\s@]+\.[^\s@]+$/,
      message: '请输入正确的邮箱格式',
      type: 'error' as const,
    },
  ],
  password: [{ required: true, message: '请输入密码', type: 'error' as const }],
}

// 登录处理
const handleSubmit: FormProps['onSubmit'] = async ({ validateResult }) => {
  if (validateResult === true && !loading.value) {
    loading.value = true
    try {
      await userStore.loginByEmail(formData.email.trim(), formData.password)

      MessagePlugin.success('登录成功')
      await router.replace(getLoginRedirect(route.query.redirect))
    } catch (error) {
      console.error('登录失败:', error)
      // 错误信息已经在request.ts中处理了，这里不需要再次显示
    } finally {
      loading.value = false
    }
  }
}

onMounted(() => {
  loadScript(canvasURL)
})

onBeforeUnmount(() => {
  removeScript(canvasURL)
  // 销毁 canvas-nest 实例
  if (window.CanvasNest) {
    window.CanvasNest.destroy()
    window.CanvasNest = undefined
  }
})
</script>

<style scoped>
.login-container {
  min-height: 100vh;
  background: #f0f2f5;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 20px;
}

.login-card {
  background: white;
  border-radius: 12px;
  box-shadow: 0 20px 40px rgba(0, 0, 0, 0.1);
  padding: 40px;
  width: 100%;
  max-width: 400px;
}

.login-header {
  text-align: center;
  margin-bottom: 32px;
}

.login-header h2 {
  color: #333;
  font-size: 28px;
  font-weight: 600;
  margin: 0 0 8px 0;
}

.login-header p {
  color: #666;
  font-size: 14px;
  margin: 0;
}

.login-form {
  width: 100%;
}

.login-form :deep(.t-form-item) {
  margin-bottom: 20px;
}

.login-form :deep(.t-form-item:last-child) {
  margin-bottom: 0;
}

.login-form :deep(.t-input) {
  height: 48px;
}

.login-form :deep(.t-button) {
  height: 48px;
  font-size: 16px;
  border-radius: 6px;
}

/* 响应式设计 */
@media (max-width: 480px) {
  .login-card {
    padding: 24px;
    margin: 0 16px;
  }

  .login-header h2 {
    font-size: 24px;
  }
}
</style>
