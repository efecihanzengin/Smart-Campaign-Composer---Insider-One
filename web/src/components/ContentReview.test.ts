import { afterEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount, type DOMWrapper, type VueWrapper } from '@vue/test-utils'
import { createPinia } from 'pinia'
import CampaignComposer from './CampaignComposer.vue'

const campaign = {
  id: 7,
  status: 'draft',
  brief: 'Sepet hatırlatması gönder',
  segment: { rule: { all: [] }, labels: ['Şehir: İstanbul'], unsupported_criteria: [] },
  audience: { sms: 412, email: 1534 },
}

const sms = { body: 'Merhaba {{first_name}}, sepetin seni bekliyor!' }
const email = {
  subject: '{{first_name}}, %30 indirim seni bekliyor',
  preheader: 'Sepetin hâlâ orada',
  body: 'Merhaba {{first_name}},\n\nSiparişini tamamla.',
  cta_text: 'Siparişi tamamla',
}
const valid = { valid: true, issues: [] }

const frame = (event: string, data: unknown) => `event: ${event}\ndata: ${JSON.stringify(data)}\n\n`

type Validator = (channel: string, content: Record<string, string>) => unknown

// The validate and edit endpoints answer with this unless a test passes its own.
const alwaysValid: Validator = () => valid

// mockApi answers the campaign, preview, channel and Content endpoints, and
// returns a controller for the Content stream so a test decides when each
// chunk arrives.
function mockApi(previewHtml = '<p>önizleme</p>', validator: Validator = alwaysValid) {
  let stream!: ReadableStreamDefaultController<Uint8Array>
  const body = new ReadableStream<Uint8Array>({ start: (c) => void (stream = c) })
  const fetchMock = vi.fn(async (url: string, init?: RequestInit) => {
    const json = (b: unknown) => new Response(JSON.stringify(b), { headers: { 'Content-Type': 'application/json' } })
    if (url === '/api/campaigns') return json(campaign)
    if (url === '/api/campaigns/7/content') return new Response(body, { headers: { 'Content-Type': 'text/event-stream' } })
    if (url === '/api/email/preview') return new Response(previewHtml, { headers: { 'Content-Type': 'text/html' } })
    const content = url.match(/^\/api\/campaigns\/7\/content\/(\w+)(\/validate)?$/)
    if (content && content[2] && init?.method === 'POST') {
      return json(validator(content[1], JSON.parse(String(init.body))))
    }
    if (content && init?.method === 'PUT') {
      const edited = JSON.parse(String(init.body))
      return json({ content: edited, validation: validator(content[1], edited) })
    }
    const channel = url.match(/^\/api\/campaigns\/7\/channels\/(\w+)$/)
    if (channel && init?.method === 'PUT') {
      const enabled = JSON.parse(String(init.body)).enabled
      return json({ enabled_channels: enabled ? ['sms', 'email'] : ['sms'] })
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
  await wrapper.get('[data-test="brief"]').setValue(campaign.brief)
  await wrapper.get('[data-test="brief-form"]').trigger('submit')
  await flushPromises()
  return wrapper
}

const value = (wrapper: VueWrapper, test: string) =>
  (wrapper.get(`[data-test="${test}"]`).element as HTMLInputElement | HTMLTextAreaElement).value

const calls = (api: ReturnType<typeof mockApi>, url: string, method: string) =>
  api.fetchMock.mock.calls.filter(([u, init]) => u === url && init?.method === method)

// type changes a field the way typing does: input events only, no change
// event until the Marketer leaves the field.
async function type(field: Pick<DOMWrapper<Element>, 'element' | 'trigger'>, text: string) {
  ;(field.element as HTMLInputElement | HTMLTextAreaElement).value = text
  await field.trigger('input')
}

afterEach(() => {
  vi.unstubAllGlobals()
  vi.useRealTimers()
})

describe('Content review', () => {
  it('fills in the SMS and email Content as it streams', async () => {
    const api = mockApi()
    const wrapper = await submit()

    await api.send('content.delta', { channels: ['sms', 'email'], sms: { body: 'Merhaba {{first' } })
    expect(wrapper.find('[data-test="content-streaming"]').exists()).toBe(true)
    expect(value(wrapper, 'sms-body')).toBe('Merhaba {{first')

    await api.send('content.delta', { channels: ['sms', 'email'], sms, email: { subject: '{{first_name}}, %30' } })
    expect(value(wrapper, 'sms-body')).toBe(sms.body)
    expect(value(wrapper, 'email-subject')).toBe('{{first_name}}, %30')

    await api.send('content.done', {
      enabled_channels: ['sms', 'email'],
      content: { sms, email },
      validation: { sms: valid, email: valid },
    })
    await api.end()

    expect(wrapper.find('[data-test="content-streaming"]').exists()).toBe(false)
    expect(value(wrapper, 'email-subject')).toBe(email.subject)
    expect(value(wrapper, 'email-preheader')).toBe(email.preheader)
    expect(value(wrapper, 'email-cta')).toBe(email.cta_text)
    expect(wrapper.find('[data-test="sms-valid"]').exists()).toBe(true)
    expect(wrapper.find('[data-test="email-valid"]').exists()).toBe(true)
  })

  it('shows the email preview rendered by the backend', async () => {
    const api = mockApi('<html><body><p>Merhaba Ayşe,</p></body></html>')
    const wrapper = await submit()

    await api.send('content.done', { enabled_channels: ['email'], content: { email }, validation: { email: valid } })
    await api.end()

    expect(JSON.parse(String(api.fetchMock.mock.calls.find(([u]) => u === '/api/email/preview')![1]!.body))).toEqual(email)
    expect(wrapper.get('[data-test="email-preview"]').attributes('srcdoc')).toContain('Merhaba Ayşe,')
  })

  it('lists validation issues per Channel', async () => {
    const api = mockApi()
    const wrapper = await submit()

    await api.send('content.done', {
      enabled_channels: ['sms', 'email'],
      content: { sms: { body: 'Merhaba {{name}}' }, email },
      validation: {
        sms: { valid: false, issues: [{ field: 'body', message: 'Bilinmeyen yer tutucu {{name}}' }] },
        email: { valid: false, issues: [{ field: 'subject', message: 'Konu en fazla 80 karakter olabilir (şu an 81).' }] },
      },
    })
    await api.end()

    expect(wrapper.get('[data-test="sms-issues"]').text()).toContain('Bilinmeyen yer tutucu {{name}}')
    expect(wrapper.get('[data-test="email-issues"]').text()).toContain('en fazla 80 karakter')
    expect(wrapper.find('[data-test="sms-valid"]').exists()).toBe(false)
  })

  it('enables only the Channels Claude inferred', async () => {
    const api = mockApi()
    const wrapper = await submit()

    await api.send('content.done', { enabled_channels: ['sms'], content: { sms }, validation: { sms: valid } })
    await api.end()

    expect((wrapper.get('[data-test="channel-sms"]').element as HTMLInputElement).checked).toBe(true)
    expect((wrapper.get('[data-test="channel-email"]').element as HTMLInputElement).checked).toBe(false)
  })

  it('lets the Marketer toggle a Channel', async () => {
    const api = mockApi()
    const wrapper = await submit()
    await api.send('content.done', {
      enabled_channels: ['sms', 'email'],
      content: { sms, email },
      validation: { sms: valid, email: valid },
    })
    await api.end()

    await wrapper.get('[data-test="channel-email"]').setValue(false)
    await flushPromises()

    const put = api.fetchMock.mock.calls.find(([u]) => u === '/api/campaigns/7/channels/email')!
    expect(put[1]!.method).toBe('PUT')
    expect(JSON.parse(String(put[1]!.body))).toEqual({ enabled: false })
    expect((wrapper.get('[data-test="channel-email"]').element as HTMLInputElement).checked).toBe(false)
    expect((wrapper.get('[data-test="channel-sms"]').element as HTMLInputElement).checked).toBe(true)
  })

  it('does not allow toggling Channels while Content streams', async () => {
    const api = mockApi()
    const wrapper = await submit()

    await api.send('content.delta', { channels: ['sms', 'email'], sms: { body: 'Mer' } })

    expect(wrapper.get('[data-test="channel-sms"]').attributes('disabled')).toBeDefined()
  })

  it('shows the error when Content generation fails', async () => {
    const api = mockApi()
    const wrapper = await submit()

    await api.send('error', { error: 'Model geçersiz bir içerik üretti. Lütfen tekrar deneyin.' })
    await api.end()

    expect(wrapper.get('[data-test="content-error"]').text()).toContain('geçersiz bir içerik')
  })
})

const usage = {
  sms: { valid: true, issues: [], usage: { body: { encoding: 'gsm7', used: 142, limit: 160 } } },
  email: {
    valid: true,
    issues: [],
    usage: {
      subject: { used: 23, limit: 80 },
      preheader: { used: 18, limit: 120 },
      body: { used: 640, limit: 1500 },
    },
  },
}

// A stand-in for the backend validator: any ş makes the SMS UCS-2.
const turkishIsUCS2: Validator = (channel, c) => {
  if (channel !== 'sms') return usage.email
  return c.body.includes('ş')
    ? {
        valid: false,
        issues: [{ field: 'body', message: 'SMS tek parçaya sığmalı (şu an 71).' }],
        usage: { body: { encoding: 'ucs2', used: 71, limit: 70 } },
      }
    : { valid: true, issues: [], usage: { body: { encoding: 'gsm7', used: c.body.length, limit: 160 } } }
}

async function reviewed(validator: Validator = alwaysValid) {
  const api = mockApi('<p>önizleme</p>', validator)
  const wrapper = await submit()
  await api.send('content.done', { enabled_channels: ['sms', 'email'], content: { sms, email }, validation: usage })
  await api.end()
  return { api, wrapper }
}

describe('Content counters and editing', () => {
  it('shows the SMS encoding and usage and the email limits', async () => {
    const { wrapper } = await reviewed()

    expect(wrapper.get('[data-test="sms-counter"]').text()).toBe('GSM-7 · 142/160')
    expect(wrapper.get('[data-test="email-subject-counter"]').text()).toBe('23/80')
    expect(wrapper.get('[data-test="email-preheader-counter"]').text()).toBe('18/120')
    expect(wrapper.get('[data-test="email-body-counter"]').text()).toBe('640/1500')
  })

  it('updates the counter as the Marketer types, asking the backend validator once typing pauses', async () => {
    const { api, wrapper } = await reviewed(turkishIsUCS2)
    vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout'] })

    const body = wrapper.get('[data-test="sms-body"]')
    await type(body, 'Merhaba Ay')
    await type(body, 'Merhaba Ayşe')
    expect(calls(api, '/api/campaigns/7/content/sms/validate', 'POST')).toHaveLength(0)

    await vi.advanceTimersByTimeAsync(500)
    await flushPromises()

    const validated = calls(api, '/api/campaigns/7/content/sms/validate', 'POST')
    expect(validated).toHaveLength(1)
    expect(JSON.parse(String(validated[0][1]!.body))).toEqual({ body: 'Merhaba Ayşe' })
    expect(wrapper.get('[data-test="sms-counter"]').text()).toBe('UCS-2 · 71/70')
    expect(wrapper.get('[data-test="sms-issues"]').text()).toContain('tek parçaya sığmalı')
  })

  it('validates email edits with the backend validator', async () => {
    const { api, wrapper } = await reviewed(() => ({
      valid: false,
      issues: [{ field: 'subject', message: 'Konu en fazla 80 karakter olabilir (şu an 81).' }],
      usage: { ...usage.email.usage, subject: { used: 81, limit: 80 } },
    }))
    vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout'] })

    await type(wrapper.get('[data-test="email-subject"]'), 'x'.repeat(81))
    await vi.advanceTimersByTimeAsync(500)
    await flushPromises()

    const validated = calls(api, '/api/campaigns/7/content/email/validate', 'POST')
    expect(validated).toHaveLength(1)
    expect(JSON.parse(String(validated[0][1]!.body))).toEqual({ ...email, subject: 'x'.repeat(81) })
    expect(wrapper.get('[data-test="email-subject-counter"]').text()).toBe('81/80')
    expect(wrapper.get('[data-test="email-issues"]').text()).toContain('en fazla 80 karakter')
  })

  it('saves an edit when the Marketer leaves the field', async () => {
    const { api, wrapper } = await reviewed(turkishIsUCS2)

    const body = wrapper.get('[data-test="sms-body"]')
    await type(body, 'Merhaba Ayşe')
    await body.trigger('change')
    await flushPromises()

    const saved = calls(api, '/api/campaigns/7/content/sms', 'PUT')
    expect(saved).toHaveLength(1)
    expect(JSON.parse(String(saved[0][1]!.body))).toEqual({ body: 'Merhaba Ayşe' })
    expect(wrapper.get('[data-test="sms-counter"]').text()).toBe('UCS-2 · 71/70')
  })

  it('does not allow editing while Content streams', async () => {
    const api = mockApi()
    const wrapper = await submit()

    await api.send('content.delta', { channels: ['sms', 'email'], sms: { body: 'Mer' }, email: { subject: 'Ko' } })

    for (const field of ['sms-body', 'email-subject']) {
      expect(wrapper.get(`[data-test="${field}"]`).attributes('disabled'), field).toBeDefined()
    }

    await api.send('content.done', { enabled_channels: ['sms', 'email'], content: { sms, email }, validation: usage })
    await api.end()

    for (const field of ['sms-body', 'email-subject', 'email-preheader', 'email-body', 'email-cta']) {
      expect(wrapper.get(`[data-test="${field}"]`).attributes('disabled'), field).toBeUndefined()
    }
  })
})
