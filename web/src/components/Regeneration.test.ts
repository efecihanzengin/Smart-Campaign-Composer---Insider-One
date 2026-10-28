import { afterEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { createPinia } from 'pinia'
import CampaignComposer from './CampaignComposer.vue'
import { openStream, sse } from '../testing/sse'

const campaign = {
  id: 7,
  status: 'draft',
  brief: 'Sepet hatırlatması gönder',
  segment: { rule: { all: [] }, labels: ['Şehir: Konya'], unsupported_criteria: [] },
  audience: { sms: 412, email: 1534 },
}

const sms = { body: '{{city}} için sepetin seni bekliyor!' }
const email = {
  subject: '{{first_name}}, %30 indirim seni bekliyor',
  preheader: 'Sepetin hâlâ orada',
  body: 'Merhaba {{first_name}},\n\nSiparişini tamamla.',
  cta_text: 'Siparişi tamamla',
}
const valid = { valid: true, issues: [] }
const tooLong = {
  valid: false,
  issues: [{ field: 'body', message: 'SMS tek parçaya sığmalı: GSM-7 ile en fazla 160 karakter olabilir (şu an 165).' }],
  usage: { body: { encoding: 'gsm7', used: 165, limit: 160 } },
}

const frame = (event: string, data: unknown) => `event: ${event}\ndata: ${JSON.stringify(data)}\n\n`
const json = (b: unknown, status = 200) =>
  new Response(JSON.stringify(b), { status, headers: { 'Content-Type': 'application/json' } })

// mockApi answers every endpoint the review screen uses. Each streamed POST
// (first Content call or a Channel's regeneration) opens a new stream that
// the test feeds with send(); segment() sets the next Segment regeneration answer.
function mockApi() {
  let stream!: ReadableStreamDefaultController<Uint8Array>
  let segmentAnswer: () => Response = () => json({})
  const fetchMock = vi.fn(async (url: string, init?: RequestInit) => {
    if (url === '/api/campaigns') return sse(['segment.done', campaign])
    if (url === '/api/campaigns/7/content' || /^\/api\/campaigns\/7\/content\/\w+\/regenerate$/.test(url)) {
      const body = new ReadableStream<Uint8Array>({ start: (c) => void (stream = c) })
      return new Response(body, { headers: { 'Content-Type': 'text/event-stream' } })
    }
    if (url === '/api/campaigns/7/segment/regenerate') return segmentAnswer()
    if (url === '/api/email/preview') return new Response('<p>önizleme</p>', { headers: { 'Content-Type': 'text/html' } })
    const content = url.match(/^\/api\/campaigns\/7\/content\/(\w+)(\/validate)?$/)
    if (content && content[2]) return json(valid)
    if (content && init?.method === 'PUT') return json({ content: JSON.parse(String(init.body)), validation: valid })
    return new Response('not found', { status: 404 })
  })
  vi.stubGlobal('fetch', fetchMock)
  const encoder = new TextEncoder()
  return {
    fetchMock,
    segment: (answer: () => Response) => void (segmentAnswer = answer),
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

async function reviewed() {
  const api = mockApi()
  const wrapper = mount(CampaignComposer, { global: { plugins: [createPinia()] } })
  await wrapper.get('[data-test="brief"]').setValue(campaign.brief)
  await wrapper.get('[data-test="brief-form"]').trigger('submit')
  await flushPromises()
  await api.send('content.done', {
    enabled_channels: ['sms', 'email'],
    content: { sms, email },
    validation: { sms: valid, email: valid },
  })
  await api.end()
  return { api, wrapper }
}

const value = (wrapper: VueWrapper, test: string) =>
  (wrapper.get(`[data-test="${test}"]`).element as HTMLInputElement | HTMLTextAreaElement).value

const calls = (api: ReturnType<typeof mockApi>, url: string) =>
  api.fetchMock.mock.calls.filter(([u, init]) => u === url && init?.method === 'POST')

async function regenerate(wrapper: VueWrapper, part: 'sms' | 'email' | 'segment', instruction: string) {
  await wrapper.get(`[data-test="${part}-instruction"]`).setValue(instruction)
  await wrapper.get(`[data-test="${part}-regenerate"]`).trigger('click')
  await flushPromises()
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('Regenerating one Channel', () => {
  it('sends the instruction and streams only into that Channel, keeping the other', async () => {
    const { api, wrapper } = await reviewed()
    // The Marketer has edited the email without leaving the field yet.
    const subject = wrapper.get('[data-test="email-subject"]')
    ;(subject.element as HTMLInputElement).value = 'Elle yazılmış konu'
    await subject.trigger('input')

    await regenerate(wrapper, 'sms', 'daha kısa')

    const posted = calls(api, '/api/campaigns/7/content/sms/regenerate')
    expect(posted).toHaveLength(1)
    expect(JSON.parse(String(posted[0][1]!.body))).toEqual({ instruction: 'daha kısa' })
    expect(wrapper.find('[data-test="sms-regenerating"]').exists()).toBe(true)
    expect(wrapper.find('[data-test="email-regenerating"]').exists()).toBe(false)

    await api.send('content.delta', { channels: ['sms'], sms: { body: 'Kısa' } })
    expect(value(wrapper, 'sms-body')).toBe('Kısa')
    expect(value(wrapper, 'email-subject')).toBe('Elle yazılmış konu')

    await api.send('content.done', {
      enabled_channels: ['sms', 'email'],
      content: { sms: { body: 'Kısa metin' }, email },
      validation: { sms: valid, email: valid },
      repairs: 0,
    })
    await api.end()

    expect(value(wrapper, 'sms-body')).toBe('Kısa metin')
    expect(value(wrapper, 'email-subject')).toBe('Elle yazılmış konu')
    expect(wrapper.find('[data-test="sms-regenerating"]').exists()).toBe(false)
    expect(calls(api, '/api/campaigns/7/content')).toHaveLength(1)
  })

  it('cannot be started while Content streams', async () => {
    const { api, wrapper } = await reviewed()
    await regenerate(wrapper, 'email', '')

    for (const button of ['sms-regenerate', 'email-regenerate', 'segment-regenerate']) {
      expect(wrapper.get(`[data-test="${button}"]`).attributes('disabled'), button).toBeDefined()
    }
    await api.end()
  })

  it('keeps the current Content when the regeneration fails, and retries the same regeneration', async () => {
    const { api, wrapper } = await reviewed()
    await regenerate(wrapper, 'sms', 'daha kısa')
    await api.send('content.delta', { channels: ['sms'], sms: { body: 'Merhaba {{last_name}}' } })
    await api.send('content.repair', { repair: 1, max: 2, errors: ['sms.body: Bilinmeyen yer tutucu'] })
    expect(wrapper.get('[data-test="content-repairing"]').text()).toContain('1/2')

    await api.send('error', { error: 'Model geçerli bir içerik üretemedi. Lütfen tekrar deneyin.', retryable: true })
    await api.end()

    expect(wrapper.get('[data-test="content-error"]').text()).toContain('üretemedi')
    expect(value(wrapper, 'sms-body')).toBe(sms.body)
    expect(value(wrapper, 'email-subject')).toBe(email.subject)

    await wrapper.get('[data-test="content-retry"]').trigger('click')
    await flushPromises()
    const posted = calls(api, '/api/campaigns/7/content/sms/regenerate')
    expect(posted).toHaveLength(2)
    expect(JSON.parse(String(posted[1][1]!.body))).toEqual({ instruction: 'daha kısa' })
    expect(calls(api, '/api/campaigns/7/content')).toHaveLength(1)
    await api.end()
  })
})

describe('Regenerating the Segment Rule', () => {
  it('updates the labels and counts and re-validates the kept Content, flagging what no longer fits', async () => {
    const { api, wrapper } = await reviewed()
    api.segment(() =>
      sse([
        'segment.done',
        {
          ...campaign,
          segment: { rule: { all: [] }, labels: ['Şehir: Diyarbakır'], unsupported_criteria: ['Cinsiyet: kadın'] },
          audience: { sms: 97, email: 210 },
          repairs: 0,
          enabled_channels: ['sms', 'email'],
          content: { sms, email },
          validation: { sms: tooLong, email: valid },
        },
      ]),
    )

    await regenerate(wrapper, 'segment', 'sadece kadınlar')

    const posted = calls(api, '/api/campaigns/7/segment/regenerate')
    expect(posted).toHaveLength(1)
    expect(JSON.parse(String(posted[0][1]!.body))).toEqual({ instruction: 'sadece kadınlar' })
    expect(wrapper.findAll('[data-test="segment-label"]').map((l) => l.text())).toEqual(['Şehir: Diyarbakır'])
    expect(wrapper.get('[data-test="audience-sms"]').text()).toBe('97')
    expect(wrapper.get('[data-test="unsupported-warning"]').text()).toContain('Cinsiyet: kadın')
    expect(value(wrapper, 'sms-body')).toBe(sms.body)
    expect(wrapper.get('[data-test="sms-issues"]').text()).toContain('165')
    expect(wrapper.get('[data-test="sms-counter"]').text()).toBe('GSM-7 · 165/160')
    expect(wrapper.get('[data-test="content-revalidated"]').text()).toContain('SMS')
    expect(wrapper.find('[data-test="email-valid"]').exists()).toBe(true)
    expect(calls(api, '/api/campaigns/7/content')).toHaveLength(1)
  })

  it('saves a pending edit first, so the Content checked against the new Audience is the edited one', async () => {
    const { api, wrapper } = await reviewed()
    api.segment(() => sse(['segment.done', { ...campaign, content: {}, validation: {}, enabled_channels: ['sms', 'email'] }]))
    const body = wrapper.get('[data-test="sms-body"]')
    ;(body.element as HTMLTextAreaElement).value = 'Elle yazıldı'
    await body.trigger('input')

    await regenerate(wrapper, 'segment', '')

    const order = api.fetchMock.mock.calls.map(([u, init]) => `${init?.method} ${u}`)
    const saved = order.indexOf('PUT /api/campaigns/7/content/sms')
    expect(saved).toBeGreaterThanOrEqual(0)
    expect(saved).toBeLessThan(order.indexOf('POST /api/campaigns/7/segment/regenerate'))
    expect(value(wrapper, 'sms-body')).toBe('Elle yazıldı')
  })

  it('shows that the Segment Rule is being repaired, and keeps editing and approval locked meanwhile', async () => {
    const { api, wrapper } = await reviewed()
    const stream = openStream()
    api.segment(() => stream.response)

    await regenerate(wrapper, 'segment', 'sadece Ankara')
    await stream.send('segment.repair', { repair: 1, max: 2, errors: ['rule.all[0].field: unknown field'] })

    expect(wrapper.get('[data-test="segment-repairing"]').text()).toContain('1/2')
    expect(wrapper.get('[data-test="sms-body"]').attributes('disabled')).toBeDefined()

    await stream.send('segment.done', {
      ...campaign,
      segment: { rule: { all: [] }, labels: ['Şehir: Ankara'], unsupported_criteria: [] },
      repairs: 1,
      enabled_channels: ['sms', 'email'],
      content: { sms, email },
      validation: { sms: valid, email: valid },
    })
    await stream.end()
    expect(wrapper.find('[data-test="segment-repairing"]').exists()).toBe(false)
    expect(wrapper.findAll('[data-test="segment-label"]').map((l) => l.text())).toEqual(['Şehir: Ankara'])
  })

  it('shows a retryable error and keeps the current Segment Rule when it fails', async () => {
    const { api, wrapper } = await reviewed()
    api.segment(() =>
      sse(['error', { error: 'Model geçerli bir hedef kitle kuralı üretemedi. Lütfen tekrar deneyin.', retryable: true }]),
    )

    await regenerate(wrapper, 'segment', 'x')

    expect(wrapper.get('[data-test="segment-error"]').text()).toContain('üretemedi')
    expect(wrapper.findAll('[data-test="segment-label"]').map((l) => l.text())).toEqual(['Şehir: Konya'])
    await wrapper.get('[data-test="segment-retry"]').trigger('click')
    await flushPromises()
    expect(calls(api, '/api/campaigns/7/segment/regenerate')).toHaveLength(2)
  })
})
