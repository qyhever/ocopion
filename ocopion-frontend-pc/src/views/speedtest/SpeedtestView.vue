<template>
  <main class="vps-page">
    <section class="vps-panel">
      <header class="heading">
        <div>
          <p class="eyebrow">NETWORK / SINGLE CONNECTION</p>
          <h1>VPS 测速</h1>
          <p class="muted">从当前浏览器到应用节点，测量真实连接表现。</p>
        </div>
        <t-tag variant="light" theme="primary">{{ phaseLabels[state.phase] }}</t-tag>
      </header>
      <dl class="connection">
        <div>
          <dt>测试节点</dt>
          <dd>{{ info?.nodeName || '当前应用后端' }}</dd>
        </div>
        <div>
          <dt>客户端出口 IP</dt>
          <dd>{{ info?.clientIP || '连接后获取' }}</dd>
        </div>
        <div>
          <dt>本次连接</dt>
          <dd>{{ info?.addressFamily || 'IPv4 / IPv6' }}</dd>
        </div>
        <div>
          <dt>首次握手</dt>
          <dd>{{ state.handshake === null ? '—' : `${state.handshake.toFixed(2)} ms` }}</dd>
        </div>
      </dl>
      <div class="meter" aria-live="polite" :aria-busy="running">
        <p class="eyebrow">{{ running ? phaseLabels[state.phase] : '实时速率' }}</p>
        <div class="rate">{{ state.liveRate.toFixed(2) }}<span>Mbps</span></div>
        <div class="stage-track">
          <span
            v-for="(label, index) in stages"
            :key="label"
            :class="{ active: stageIndex >= index }"
            >{{ label }}</span
          >
        </div>
        <div class="actions">
          <t-button v-if="!running" size="large" @click="start">{{
            state.phase === 'ready' ? '开始测速' : '重新测试'
          }}</t-button
          ><t-button v-else size="large" theme="danger" variant="outline" @click="stop"
            >停止测试</t-button
          ><t-button
            size="large"
            variant="outline"
            :disabled="running || state.phase === 'ready'"
            @click="copyResults"
            >复制结果</t-button
          >
        </div>
        <p class="muted">正常网络下约 30 秒，超时可能延长。测速会消耗上下行流量。</p>
      </div>
      <t-alert v-if="state.error" theme="error" :message="state.error" />
      <div class="throughput">
        <article v-for="item in transfers" :key="item.label">
          <p class="muted">
            {{ item.label }}
            <t-tag size="small" :theme="item.value.complete ? 'success' : 'default'">{{
              item.value.complete ? '完成' : '未完成'
            }}</t-tag>
          </p>
          <strong>{{ item.value.rate.toFixed(2) }} <small>Mbps</small></strong>
          <p class="muted">
            {{ (item.value.bytes / 1000000).toFixed(2) }} MB · 实际
            {{ (item.value.elapsed / 1000).toFixed(2) }} 秒
          </p>
        </article>
      </div>
      <h2>连接延迟</h2>
      <div class="latency-grid">
        <article v-for="item in latencies" :key="item.label">
          <h3>{{ item.label }}</h3>
          <dl class="metrics">
            <div v-for="metric in item.metrics" :key="metric.label">
              <dt>{{ metric.label }}</dt>
              <dd>{{ metric.value }}</dd>
            </div>
          </dl>
          <p class="muted">成功 {{ item.value.rtts.length }} / 发送 {{ item.value.sent }}</p>
        </article>
      </div>
      <p class="footnote">
        请求失败率 = 失败或超时数 / 发送数，<b>不是 IP 层丢包率</b>。P95
        使用最近秩法；抖动为相邻成功 RTT 差值绝对值的平均值。IPv4/IPv6
        表示本次连接的地址族，结果仅反映当前浏览器到此节点的单连接表现。
      </p>
      <t-textarea
        v-if="!running && state.phase !== 'ready'"
        :value="resultText"
        readonly
        :autosize="{ minRows: 4, maxRows: 12 }"
        aria-label="测速结果文本"
      />
    </section>
  </main>
</template>
<script setup lang="ts">
import { computed } from 'vue'
import { MessagePlugin } from 'tdesign-vue-next'
import { useSpeedtest, phaseLabels } from './useSpeedtest'
import { statistics } from './engine'
const { state, info, running, start, stop, copy, resultText } = useSpeedtest()
const stages = ['连接', '空闲延迟', '下载', '上传', '汇总']
const stageIndex = computed(() =>
  ['connecting', 'idle', 'download', 'upload', 'complete'].indexOf(state.phase),
)
const transfers = computed(() => [
  { label: '↓ 下载', value: state.download },
  { label: '↑ 上传', value: state.upload },
])
const latencies = computed(() =>
  [
    { label: '空闲延迟', value: state.idle },
    { label: '下载负载延迟', value: state.downloadLatency },
    { label: '上传负载延迟', value: state.uploadLatency },
  ].map((item) => {
    const s = statistics(item.value)
    const format = (v: number | null, unit = 'ms') =>
      v === null ? '样本不足' : `${v.toFixed(2)} ${unit}`
    return {
      ...item,
      metrics: [
        { label: '中位数', value: format(s.median) },
        { label: 'P95', value: format(s.p95) },
        { label: '抖动', value: format(s.jitter) },
        { label: '请求失败率', value: format(s.failure, '%') },
      ],
    }
  }),
)
async function copyResults() {
  try {
    await copy()
    MessagePlugin.success('复制成功')
  } catch {
    MessagePlugin.error('复制失败，请手动选择结果文本复制')
  }
}
</script>
<style scoped>
.vps-page {
  padding: 24px;
  color: var(--td-text-color-primary);
}
.vps-panel {
  max-width: 1120px;
  margin: auto;
  background: var(--td-bg-color-container);
  border: 1px solid var(--td-component-border);
  border-radius: 12px;
  padding: 32px;
}
.heading {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
}
h1 {
  font-size: 28px;
  font-weight: 650;
  margin: 6px 0 10px;
}
h2 {
  font-size: 18px;
  margin: 28px 0 16px;
}
h3 {
  font-weight: 600;
  margin-bottom: 18px;
}
.eyebrow {
  font-size: 11px;
  letter-spacing: 0.14em;
  color: var(--td-brand-color);
}
.muted,
dt,
.footnote {
  color: var(--td-text-color-secondary);
  font-size: 13px;
}
.connection {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 24px;
  margin: 28px 0 0;
  padding: 24px 0;
  border-block: 1px solid var(--td-component-border);
}
dd {
  margin: 8px 0 0;
  overflow-wrap: anywhere;
}
.meter {
  text-align: center;
  padding: 38px 0 24px;
}
.rate {
  font-family: 'SFMono-Regular', Consolas, monospace;
  font-size: clamp(44px, 7vw, 80px);
  font-weight: 600;
  line-height: 1.5;
  font-variant-numeric: tabular-nums;
  letter-spacing: -0.06em;
}
.rate span {
  font-size: 16px;
  letter-spacing: 0;
  margin-left: 12px;
  color: var(--td-text-color-secondary);
}
.stage-track {
  display: flex;
  gap: 8px;
  max-width: 560px;
  margin: 12px auto 28px;
}
.stage-track span {
  flex: 1;
  padding-top: 10px;
  border-top: 3px solid var(--td-component-border);
  color: var(--td-text-color-secondary);
  font-size: 12px;
}
.stage-track .active {
  border-color: var(--td-brand-color);
  color: var(--td-brand-color);
}
.actions {
  display: flex;
  justify-content: center;
  gap: 12px;
  margin-bottom: 16px;
}
.throughput {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 16px;
}
.throughput article,
.latency-grid article {
  border: 1px solid var(--td-component-border);
  border-radius: 8px;
  padding: 22px;
}
.throughput strong {
  display: block;
  font-size: 32px;
  margin: 12px 0;
  font-variant-numeric: tabular-nums;
}
small {
  font-size: 13px;
  color: var(--td-text-color-secondary);
  font-weight: 400;
}
.latency-grid {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 16px;
}
.metrics div {
  display: flex;
  justify-content: space-between;
  gap: 8px;
  margin-bottom: 12px;
}
.metrics dd {
  margin: 0;
  font-variant-numeric: tabular-nums;
  font-size: 13px;
}
.footnote {
  line-height: 1.8;
  margin: 20px 0;
}
@media (max-width: 760px) {
  .vps-page {
    padding: 12px;
  }
  .vps-panel {
    padding: 20px 16px;
  }
  .connection {
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: 18px;
  }
  .latency-grid {
    grid-template-columns: 1fr;
  }
  .throughput {
    gap: 10px;
  }
  .throughput article {
    padding: 14px;
  }
  .throughput strong {
    font-size: 24px;
  }
  .heading {
    align-items: flex-start;
  }
}
</style>
