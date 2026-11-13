import assert from 'node:assert/strict'
import { test } from 'node:test'
import { createHarness } from './auth.test.mjs'

const reply = (code, message = '', status = 200, data) =>
  new Response(JSON.stringify({ code, message, data }), {
    status,
    headers: { 'Content-Type': 'application/json' },
  })

test('Telegram 提交成功清空正文，提交期间阻止重复发送', async () => {
  let release
  const pending = new Promise((resolve) => { release = resolve })
  const h = createHarness(async (c) => {
    assert.equal(c.url.pathname, '/ocopion/api/telegram')
    assert.equal(c.method, 'POST')
    assert.equal(c.headers.Authorization, 'Bearer access-old')
    assert.deepEqual(JSON.parse(c.body), { text: '第一行\n第二行' })
    await pending
    return reply(200)
  })
  const form = h.telegramForm()
  form.formData.text = ' \n第一行\n第二行  '
  const sending = form.handleSubmit({ validateResult: true })
  assert.equal(form.submitting.value, true)
  await form.handleSubmit({ validateResult: true })
  assert.equal(h.calls.length, 1)
  release()
  await sending
  assert.equal(form.formData.text, '')
  assert.equal(form.submitting.value, false)
  assert.deepEqual(h.messages, [{ type: 'success', message: 'Telegram 消息发送成功' }])
})

test('Telegram 发送失败保留正文，统一提示一次', async () => {
  const h = createHarness(() => reply(-1, 'Telegram 消息发送失败'))
  const form = h.telegramForm()
  form.formData.text = '保留消息'
  await form.handleSubmit({ validateResult: true })
  assert.equal(form.formData.text, '保留消息')
  assert.equal(form.submitting.value, false)
  assert.deepEqual(h.messages, [{ type: 'warning', message: 'Telegram 消息发送失败' }])
  await form.handleSubmit({ validateResult: false })
  assert.equal(h.calls.length, 1)
})

test('Telegram 登录过期刷新后重试，保持消息内容', async () => {
  const h = createHarness((c) => {
    if (c.url.pathname.endsWith('/auth/refresh')) {
      return reply(200, '', 200, { accessToken: 'access-new', refreshToken: 'refresh-new' })
    }
    assert.deepEqual(JSON.parse(c.body), { text: '待发送消息' })
    return c.headers.Authorization === 'Bearer access-new'
      ? reply(200)
      : reply(1006, '登录状态失效', 401)
  })
  const form = h.telegramForm()
  form.formData.text = '待发送消息'
  await form.handleSubmit({ validateResult: true })
  assert.equal(h.calls.length, 3)
  assert.equal(form.formData.text, '')
  assert.equal(h.store.accessToken, 'access-new')
  assert.equal(h.messages.length, 1)
  assert.equal(h.messages[0].type, 'success')
})

test('Telegram 未登录跳转登录页并保留页面目标', async () => {
  const h = createHarness(() => { throw new Error('不应请求') }, {})
  const target = await h.guard('/notification/tg')
  assert.equal(target.path, '/signin')
  assert.equal(target.query.redirect, '/notification/tg')
  assert.equal(h.calls.length, 0)
})
