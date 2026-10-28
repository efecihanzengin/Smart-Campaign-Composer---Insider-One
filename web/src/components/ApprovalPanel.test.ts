import { afterEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { createPinia } from 'pinia'
import CampaignComposer from './CampaignComposer.vue'

const sms = { body: 'Merhaba {{first_name}}, sepetin seni bekliyor!' }
const email = { subject: 'İndirim', preheader: 'Sepetin hâlâ orada', body: 'Merhaba {{first_name}}', cta_text: '' }
const valid = { valid: true, issues: [] }
const invalid = { valid: false, issues: [{ field: 'body', message: 'SMS tek parçaya sığmalı.' }] }

const frame = (event: string, data: unknown) => `event: ${event}\ndata: ${JSON.stringify(data)}\n\n`

interface Options {
  audience?: { sms: number; email: number }
  // approve answers the approval request.
  approve?: () => Response
}

const json = (b: unknown, status = 200) =>
  new Response(JSON.stringify(b), { status, headers: { 'Content-Type': 'application/json' } })

function mockApi(opts: Options = {}) {
  const audience = opts.audience ?? { sms: 412, email: 1534 }
  const enabled = (['sms', 'email'] as const).filter((c) => audience[c] > 0)
  const campaign = {
    id: 7,
    status: 'draft',
    brief: 'Sepet hatırlatması gönder',
    segment: { rule: { all: [] }, labels: ['Şehir: İstanbul'], unsupported_criteria: [] },
    audience,
    enabled_channels: enabled,
    disabled_channels: Object.fromEntries(
      (['sms', 'email'] as const)
        .filter((c) => audience[c] === 0)
        .map((c) => [c, `Bu hedef kitlede ${c === 'sms' ? 'SMS' : 'E-posta'} için izin vermiş alıcı yok, bu yüzden kanal kapatıldı.`]),
    ),
  }
  let stream!: ReadableStreamDefaultController<Uint8Array>
  const body = new ReadableStream<Uint8Array>({ start: (c) => void (stream = c) })
  const fetchMock = vi.fn(async (url: string, init?: RequestInit) => {
    if (url === '/api/campaigns') return json(campaign)
    if (url === '/api/campaigns/7/content') return new Response(body, { headers: { 'Content-Type': 'text/event-stream' } })
    if (url === '/api/email/preview') return new Response('<p>önizleme</p>', { headers: { 'Content-Type': 'text/html' } })
    if (url === '/api/campaigns/7/approve' && init?.method === 'POST') {
      return opts.approve?.() ?? json({ id: 7, status: 'approved', deliveries: { sms: audience.sms, email: audience.email } })
    }
    const content = url.match(/^\/api\/campaigns\/7\/content\/(\w+)$/)
    if (content && init?.method === 'PUT') return json({ content: JSON.parse(String(init.body)), validation: valid })
    const channel = url.match(/^\/api\/campaigns\/7\/channels\/(\w+)$/)
    if (channel && init?.method === 'PUT') {
      const on = JSON.parse(String(init.body)).enabled
      return json({ enabled_channels: on ? ['sms', 'email'] : ['sms', 'email'].filter((c) => c !== channel[1]) })
    }
    return new Response('not found', { status: 404 })
  })
  vi.stubGlobal('fetch', fetchMock)
  const encoder = new TextEncoder()
  return {
    fetchMock,
    send: async (event: string, data: unknown) => {
      stream.enqueue(encoder.encode(frame(event, data)))
      await flushPromises()
    },
    end: async () => {
      stream.close()
      await flushPromises()
    },
  }
}

async function submit() {
  const wrapper = mount(CampaignComposer, { global: { plugins: [createPinia()] } })
  await wrapper.get('[data-test="brief"]').setValue('Sepet hatırlatması gönder')
  await wrapper.get('[data-test="brief-form"]').trigger('submit')
  await flushPromises()
  return wrapper
}

async function reviewed(api: ReturnType<typeof mockApi>, done: Record<string, unknown> = {}) {
  const wrapper = await submit()
  await api.send('content.done', {
    enabled_channels: ['sms', 'email'],
    content: { sms, email },
    validation: { sms: valid, email: valid },
    ...done,
  })
  await api.end()
  return wrapper
}

const button = (w: VueWrapper) => w.get('[data-test="approve"]')
const reasons = (w: VueWrapper) => w.findAll('[data-test="approve-reason"]').map((r) => r.text())

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('Approval', () => {
  it('can be approved once every enabled Channel has valid Content and an Audience', async () => {
    const api = mockApi()
    const wrapper = await reviewed(api)

    expect(button(wrapper).attributes('disabled')).toBeUndefined()
    expect(reasons(wrapper)).toEqual([])
  })

  it('is disabled while Content is being generated', async () => {
    const api = mockApi()
    const wrapper = await submit()
    await api.send('content.delta', { channels: ['sms', 'email'], sms: { body: 'Mer' } })

    expect(button(wrapper).attributes('disabled')).toBeDefined()
    expect(reasons(wrapper)).toEqual(['İçerik hâlâ üretiliyor; bitmesini bekleyin.'])
  })

  it('is disabled while an enabled Channel has invalid Content, naming the Channel', async () => {
    const api = mockApi()
    const wrapper = await reviewed(api, { validation: { sms: invalid, email: valid } })

    expect(button(wrapper).attributes('disabled')).toBeDefined()
    expect(reasons(wrapper)).toEqual(['SMS içeriği kurallara uymuyor.'])
  })

  it('is disabled when an enabled Channel has no Content', async () => {
    const api = mockApi()
    const wrapper = await reviewed(api, { content: { sms }, validation: { sms: valid } })

    expect(reasons(wrapper)).toEqual(['E-posta içeriği yok.'])
  })

  it('ignores problems of a disabled Channel', async () => {
    const api = mockApi()
    const wrapper = await reviewed(api, { validation: { sms: invalid, email: valid } })

    await wrapper.get('[data-test="channel-sms"]').setValue(false)
    await flushPromises()

    expect(button(wrapper).attributes('disabled')).toBeUndefined()
  })

  it('is disabled with no Channel enabled', async () => {
    const api = mockApi()
    const wrapper = await reviewed(api, { enabled_channels: [] })

    expect(button(wrapper).attributes('disabled')).toBeDefined()
    expect(reasons(wrapper)).toEqual(['En az bir kanal açık olmalı.'])
  })

  it('turns off a Channel with no consenting Recipients and explains why', async () => {
    const api = mockApi({ audience: { sms: 0, email: 1534 } })
    const wrapper = await reviewed(api, { enabled_channels: ['email'] })

    const toggle = wrapper.get('[data-test="channel-sms"]')
    expect((toggle.element as HTMLInputElement).checked).toBe(false)
    expect(toggle.attributes('disabled')).toBeDefined()
    expect(wrapper.get('[data-test="channel-sms-disabled"]').text()).toContain('SMS için izin vermiş alıcı yok')
    expect(button(wrapper).attributes('disabled')).toBeUndefined()
  })

  it('keeps a Channel with no consenting Recipients off while Content streams', async () => {
    const api = mockApi({ audience: { sms: 0, email: 1534 } })
    const wrapper = await submit()
    await api.send('content.delta', { channels: ['sms', 'email'], sms: { body: 'Mer' } })

    expect((wrapper.get('[data-test="channel-sms"]').element as HTMLInputElement).checked).toBe(false)
  })

  it('approves the Campaign and shows the Deliveries created', async () => {
    const api = mockApi()
    const wrapper = await reviewed(api)

    await button(wrapper).trigger('click')
    await flushPromises()

    expect(api.fetchMock.mock.calls.filter(([u]) => u === '/api/campaigns/7/approve')).toHaveLength(1)
    expect(wrapper.get('[data-test="approved"]').text()).toContain('412 SMS')
    expect(wrapper.get('[data-test="approved"]').text()).toContain('1.534 e-posta')
    expect(wrapper.find('[data-test="approve"]').exists()).toBe(false)
    expect(wrapper.get('[data-test="sms-body"]').attributes('disabled')).toBeDefined()
    expect(wrapper.get('[data-test="channel-email"]').attributes('disabled')).toBeDefined()
  })

  it('saves an unsaved edit before approving', async () => {
    const api = mockApi()
    const wrapper = await reviewed(api)

    const body = wrapper.get('[data-test="sms-body"]')
    ;(body.element as HTMLTextAreaElement).value = 'Yeni metin'
    await body.trigger('input')
    await button(wrapper).trigger('click')
    await flushPromises()

    const urls = api.fetchMock.mock.calls.map(([u, init]) => `${init?.method ?? 'GET'} ${u}`)
    const saved = urls.indexOf('PUT /api/campaigns/7/content/sms')
    expect(saved).toBeGreaterThanOrEqual(0)
    expect(saved).toBeLessThan(urls.indexOf('POST /api/campaigns/7/approve'))
  })

  it('shows the reasons the backend refused approval for', async () => {
    const api = mockApi({
      approve: () =>
        json(
          {
            error: 'Kampanya henüz onaylanamaz.',
            reasons: [{ code: 'empty_audience', channel: 'sms', message: 'SMS kanalında izin vermiş alıcı yok; kanalı kapatın.' }],
          },
          422,
        ),
    })
    const wrapper = await reviewed(api)

    await button(wrapper).trigger('click')
    await flushPromises()

    expect(wrapper.get('[data-test="approve-error"]').text()).toContain('SMS kanalında izin vermiş alıcı yok')
    expect(wrapper.find('[data-test="approved"]').exists()).toBe(false)
  })
})
