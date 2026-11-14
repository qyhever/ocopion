<template>
  <main class="studio">
    <header class="heading">
      <div>
        <p class="eyebrow">创作工具 / 01</p>
        <h1>让文字，有声有画。</h1>
        <p class="intro">一张封面，一段文案，生成可以分享的语音视频。</p>
      </div>
      <span class="format">MP4 · 1080P · 30 FPS</span>
    </header>
    <div class="workspace">
      <section class="editor" aria-label="视频内容设置">
        <h2>文案转视频</h2>
        <t-form label-align="top" @submit="submit">
          <t-form-item label="朗读文案">
            <div class="text-field">
              <t-textarea
                v-model="text"
                :disabled="busy"
                placeholder="写下想要朗读的文字……"
                :autosize="{ minRows: 9, maxRows: 18 }"
              />
              <p class="counter" :class="{ exceeded: textLength > 10000 }">
                {{ textLength.toLocaleString() }} / 10,000 字
              </p>
            </div>
          </t-form-item>
          <t-form-item label="视频封面">
            <div class="cover-field">
              <input
                id="video-cover"
                type="file"
                accept="image/jpeg,image/png,image/webp"
                :disabled="busy"
                @change="selectCover"
              />
              <p class="hint">JPEG、PNG 或 WebP，最大 10 MiB。封面会居中显示，留白补齐至 16:9。</p>
            </div>
          </t-form-item>
          <t-form-item label="朗读倍速">
            <div>
              <t-radio-group v-model="speed" :disabled="busy" variant="default-filled">
                <t-radio-button value="1">1 倍速</t-radio-button>
                <t-radio-button value="0.5">0.5 倍速</t-radio-button>
              </t-radio-group>
              <p class="hint">0.5 倍速为半速朗读，视频时长约翻倍。</p>
            </div>
          </t-form-item>
          <t-button
            theme="primary"
            type="submit"
            :loading="submitting"
            :disabled="busy || !text.trim() || !cover || textLength > 10000"
            >生成视频</t-button
          >
        </t-form>
      </section>
      <section class="output" aria-label="视频预览与生成结果" aria-live="polite">
        <div class="output-heading">
          <h2>预览与成品</h2>
          <span>{{ statusLabel }}</span>
        </div>
        <video
          v-if="task?.status === 'succeeded' && task.videoUrl"
          :src="task.videoUrl"
          controls
          preload="metadata"
          class="preview"
        />
        <div v-else class="preview placeholder">
          <img v-if="preview" :src="preview" alt="所选封面预览" />
          <div v-else class="empty-preview">
            <span class="frame-mark">▶</span>
            <p>你的故事，从这里开始</p>
            <small>选择封面后预览画面</small>
          </div>
        </div>
        <div v-if="task?.status === 'running'" class="status-box">
          <strong>正在为文案配音并合成视频</strong>
          <p>你可以稍后回来查看结果，刷新页面也能恢复查询。</p>
        </div>
        <div v-if="queryError" class="status-box error">
          <p>{{ queryError }}</p>
          <t-button variant="outline" :loading="querying" @click="poll">恢复查询</t-button>
        </div>
        <div v-if="task?.status === 'failed'" class="status-box error">
          <strong>生成失败</strong>
          <p>{{ task.error }}</p>
          <p>调整内容后可重新生成。</p>
        </div>
        <p v-if="notice" class="status-box">{{ notice }}</p>
        <div v-if="task?.status === 'succeeded'" class="result">
          <label for="video-address">视频地址</label>
          <t-input id="video-address" :value="fullVideoUrl" readonly />
          <div class="actions">
            <t-button variant="outline" @click="copyAddress">复制地址</t-button>
            <a :href="task.downloadUrl" class="download"
              >下载 MP4 <span aria-hidden="true">↓</span></a
            >
          </div>
        </div>
        <p class="footnote">中文女声 · 晓晓<br />生成时间随文案长度变化，请耐心等待。</p>
      </section>
    </div>
  </main>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { MessagePlugin } from 'tdesign-vue-next'
import { useUserStore } from '@/stores/user'
import { createVideoTask, getVideoTask, type VideoTask } from './service'

const userStore = useUserStore()
const storageKey = `txt-video:last-task:${userStore.userInfo!.id}`
const session = userStore.sessionVersion
const text = ref('')
const speed = ref('1')
const cover = ref<File>()
const preview = ref('')
const task = ref<VideoTask>()
const submitting = ref(false)
const querying = ref(false)
const queryError = ref('')
const notice = ref('')
let timer: ReturnType<typeof setTimeout> | undefined
let disposed = false
const current = () => !disposed && session === userStore.sessionVersion
const busy = computed(() => submitting.value || task.value?.status === 'running')
const textLength = computed(() => Array.from(text.value.trim()).length)
const statusLabel = computed(() =>
  task.value
    ? { running: '生成中', succeeded: '已完成', failed: '生成失败' }[task.value.status]
    : '等待创作',
)
const fullVideoUrl = computed(() =>
  task.value?.videoUrl ? new URL(task.value.videoUrl, window.location.origin).href : '',
)
function saveTask(id?: string) {
  try {
    if (id) localStorage.setItem(storageKey, id)
    else localStorage.removeItem(storageKey)
  } catch {
    /* 浏览器禁用存储时仍可在当前页使用。 */
  }
}
function selectCover(event: Event) {
  const input = event.target as HTMLInputElement
  const file = input.files?.[0]
  if (!file) return
  if (
    !['image/jpeg', 'image/png', 'image/webp'].includes(file.type) ||
    file.size > 10 * 1024 * 1024
  ) {
    input.value = ''
    MessagePlugin.warning('请选择不超过 10 MiB 的 JPEG、PNG 或 WebP 图片')
    return
  }
  if (preview.value) URL.revokeObjectURL(preview.value)
  cover.value = file
  preview.value = URL.createObjectURL(file)
}
async function poll() {
  clearTimeout(timer)
  if (!current() || !task.value || querying.value) return
  querying.value = true
  queryError.value = ''
  try {
    const result = await getVideoTask(task.value.id)
    if (!current()) return
    task.value = result
    if (result.status === 'running') timer = setTimeout(poll, 2000)
  } catch (cause) {
    if (!current()) return
    const error = cause as { data?: { code?: number } }
    if (error.data?.code === 404) {
      saveTask()
      task.value = undefined
      notice.value = '任务不存在或已过期，请重新生成。'
    } else {
      queryError.value = '查询暂时中断，任务已保留。请恢复查询后查看结果。'
    }
  } finally {
    querying.value = false
  }
}
async function submit() {
  if (busy.value || !cover.value || !text.value.trim() || textLength.value > 10000) return
  submitting.value = true
  notice.value = ''
  queryError.value = ''
  try {
    const result = await createVideoTask(text.value.trim(), cover.value, speed.value)
    if (!current()) return
    task.value = result
    saveTask(result.id)
    timer = setTimeout(poll, 2000)
  } catch {
    /* 请求层统一提示；保留表单以便重试。 */
  } finally {
    submitting.value = false
  }
}
async function copyAddress() {
  try {
    await navigator.clipboard.writeText(fullVideoUrl.value)
    MessagePlugin.success('视频地址已复制')
  } catch {
    MessagePlugin.warning('复制失败，请选中上方地址手动复制')
  }
}
onMounted(() => {
  let id: string | null = null
  try {
    id = localStorage.getItem(storageKey)
  } catch {
    /* 存储可能不可用。 */
  }
  if (id) {
    task.value = { id, status: 'running', error: '' }
    void poll()
  }
})
onUnmounted(() => {
  disposed = true
  clearTimeout(timer)
  if (preview.value) URL.revokeObjectURL(preview.value)
})
</script>

<style scoped>
.studio {
  max-width: 1320px;
  margin: 0 auto;
  padding: 36px 28px;
  color: #25322f;
}
.heading {
  display: flex;
  justify-content: space-between;
  align-items: flex-end;
  gap: 20px;
  margin-bottom: 30px;
}
.eyebrow {
  font-size: 12px;
  letter-spacing: 0.12em;
  color: #647c71;
  margin: 0 0 12px;
}
h1 {
  font-family: 'Songti SC', 'SimSun', serif;
  font-size: clamp(26px, 3vw, 38px);
  font-weight: 600;
  margin: 0 0 12px;
}
.intro {
  color: #66726e;
  font-size: 14px;
  margin: 0;
}
.format {
  font-size: 11px;
  letter-spacing: 0.08em;
  color: #66726e;
  white-space: nowrap;
}
.workspace {
  display: grid;
  grid-template-columns: 1.12fr 1fr;
  background: white;
  border: 1px solid #e2e7e4;
  border-radius: 12px;
  overflow: hidden;
}
.editor,
.output {
  padding: 30px;
  min-width: 0;
}
.editor {
  border-right: 1px solid #e2e7e4;
}
.output {
  background: #f7f9f7;
}
h2 {
  margin: 0 0 26px;
  font-size: 17px;
  font-weight: 600;
}
.text-field,
.cover-field {
  width: 100%;
}
.counter {
  text-align: right;
  margin: 8px 0 0;
  font-size: 12px;
  color: #7a8580;
}
.exceeded,
.error {
  color: #b13d31;
}
.hint,
.footnote {
  color: #748079;
  font-size: 12px;
  line-height: 1.8;
  margin: 10px 0 0;
}
input[type='file'] {
  max-width: 100%;
  font-size: 13px;
}
input::file-selector-button {
  border: 1px solid #d1dad4;
  background: #f7f9f7;
  padding: 9px 16px;
  border-radius: 5px;
  margin-right: 12px;
  cursor: pointer;
}
.output-heading {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
}
.output-heading span {
  font-size: 12px;
  color: #647c71;
}
.preview {
  display: block;
  width: 100%;
  aspect-ratio: 16/9;
  background: #182923;
  border-radius: 6px;
  object-fit: contain;
  overflow: hidden;
}
.placeholder {
  display: grid;
  place-items: center;
}
.preview img {
  width: 100%;
  height: 100%;
  object-fit: contain;
}
.empty-preview {
  color: #c3d0c8;
  text-align: center;
}
.frame-mark {
  display: inline-block;
  font-size: 24px;
  margin-bottom: 8px;
  color: #88a898;
}
.empty-preview p {
  font-family: 'Songti SC', serif;
  margin: 0 0 8px;
  font-size: 18px;
}
.empty-preview small {
  color: #8fa69a;
}
.status-box {
  margin: 20px 0 0;
  padding: 16px;
  background: #edf1ed;
  border-left: 3px solid #7d9c89;
  font-size: 13px;
  line-height: 1.8;
  overflow-wrap: anywhere;
}
.status-box p {
  margin: 4px 0;
  white-space: pre-wrap;
}
.error {
  background: #fff1ee;
  border-color: #bd6254;
}
.result {
  margin-top: 24px;
}
.result label {
  display: block;
  font-size: 13px;
  margin-bottom: 10px;
}
.actions {
  display: flex;
  gap: 12px;
  margin-top: 14px;
}
.download {
  display: inline-flex;
  align-items: center;
  gap: 22px;
  background: #254d3e;
  color: white;
  padding: 6px 16px;
  border-radius: 3px;
  text-decoration: none;
  font-size: 14px;
}
.download:hover {
  background: #356a56;
}
.footnote {
  margin-top: 26px;
}
@media (max-width: 900px) {
  .workspace {
    grid-template-columns: 1fr;
  }
  .editor {
    border-right: 0;
    border-bottom: 1px solid #e2e7e4;
  }
  .format {
    display: none;
  }
  .studio {
    padding: 24px 16px;
  }
  .editor,
  .output {
    padding: 22px;
  }
}
</style>
