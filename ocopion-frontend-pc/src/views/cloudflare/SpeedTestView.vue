<template>
  <main class="speed-test-page p-5">
    <section class="speed-test-panel">
      <header class="page-heading">
        <div>
          <p class="eyebrow">CLOUDFLARE / IPv4</p>
          <h1>Cloudflare 优选</h1>
          <p class="description">
            从服务器网络测试 Cloudflare IPv4 延迟，筛选本次测速中表现最佳的 12 个地址。
          </p>
        </div>
        <t-button theme="primary" :loading="loading" :disabled="loading" @click="startTest">
          {{ loading ? '正在测速' : '开始测速' }}
        </t-button>
      </header>

      <dl class="parameters">
        <div>
          <dt>延迟上限</dt>
          <dd>200 <small>ms</small></dd>
        </div>
        <div>
          <dt>返回数量</dt>
          <dd>12 <small>个 IPv4</small></dd>
        </div>
        <div>
          <dt>测速方式</dt>
          <dd>TCP <small>延迟 · 不测下载</small></dd>
        </div>
      </dl>
      <p class="wait-note">测速最长需要 10 分钟，请保持页面打开。结果反映服务器网络状况。</p>

      <div class="result-heading">
        <h2>优选结果</h2>
        <div class="result-actions">
          <span>{{ ips.length }} / 12</span>
          <t-button
            size="small"
            variant="outline"
            :disabled="!ips.length || loading"
            @click="copyResults"
          >
            复制
          </t-button>
        </div>
      </div>
      <div aria-live="polite" :aria-busy="loading">
        <t-alert
          v-if="loading"
          theme="info"
          message="正在测速，请稍候… 完成后将自动显示 IP 列表。"
        />
        <t-alert
          v-else-if="failed"
          theme="error"
          message="本次测速失败，请查看错误提示后重新测速。"
        />
        <t-textarea
          v-else-if="ips.length"
          class="ip-results"
          :value="resultText"
          :autosize="{ minRows: 6, maxRows: 12 }"
          readonly
          aria-label="优选 IP 列表"
        />
        <div v-else class="empty-state">尚无测速结果，点击「开始测速」获取优选 IP。</div>
      </div>
    </section>
  </main>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { MessagePlugin } from 'tdesign-vue-next'
import { speedTest } from '@/api/cloudflare'

const loading = ref(false)
const failed = ref(false)
const ips = ref<string[]>([])
const resultText = computed(() => ips.value.join('\n'))

async function copyResults() {
  if (!resultText.value || loading.value) return
  try {
    await navigator.clipboard.writeText(resultText.value)
    MessagePlugin.success('复制成功')
  } catch {
    MessagePlugin.error('复制失败，请手动选择文本域内容复制')
  }
}

async function startTest() {
  if (loading.value) return
  ips.value = []
  failed.value = false
  loading.value = true
  try {
    const result = await speedTest()
    ips.value = result.ips
  } catch {
    failed.value = true
    // 详细错误由统一请求层提示。
  } finally {
    loading.value = false
  }
}
</script>

<style scoped>
.speed-test-panel {
  max-width: 1080px;
  padding: 32px;
  background: var(--td-bg-color-container);
  border-radius: var(--td-radius-large);
}
.page-heading {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 24px;
}
.eyebrow {
  margin: 0 0 8px;
  color: var(--td-brand-color);
  font-size: 12px;
  letter-spacing: 0.12em;
}
h1 {
  margin: 0;
  font-size: 24px;
  font-weight: 600;
}
.description,
.wait-note {
  color: var(--td-text-color-secondary);
  font-size: 14px;
  line-height: 1.7;
}
.parameters {
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  margin: 28px 0 12px;
  padding: 24px;
  gap: 24px;
  background: var(--td-bg-color-secondarycontainer);
  border-radius: var(--td-radius-default);
}
dt {
  color: var(--td-text-color-secondary);
  font-size: 13px;
}
dd {
  margin: 8px 0 0;
  font-size: 26px;
  font-weight: 600;
  font-variant-numeric: tabular-nums;
}
small {
  font-size: 13px;
  font-weight: 400;
  color: var(--td-text-color-secondary);
}
.result-heading {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin: 32px 0 16px;
}
h2 {
  margin: 0;
  font-size: 16px;
  font-weight: 600;
}
.result-actions {
  display: flex;
  align-items: center;
  gap: 12px;
}
.result-actions > span {
  color: var(--td-text-color-secondary);
  font-variant-numeric: tabular-nums;
}
.ip-results :deep(textarea) {
  font-family: 'SFMono-Regular', Consolas, monospace;
  font-size: 16px;
  line-height: 1.7;
}
.empty-state {
  padding: 48px 16px;
  text-align: center;
  color: var(--td-text-color-secondary);
  border: 1px dashed var(--td-component-stroke);
  border-radius: var(--td-radius-default);
}
@media (max-width: 760px) {
  .speed-test-panel {
    padding: 20px;
  }
  .page-heading {
    align-items: flex-start;
    flex-direction: column;
  }
  .parameters {
    grid-template-columns: 1fr;
    gap: 16px;
  }
}
</style>
