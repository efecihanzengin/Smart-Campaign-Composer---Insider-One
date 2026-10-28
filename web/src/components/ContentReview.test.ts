import { afterEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
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

// mockApi answers the campaign, preview and channel endpoints, and returns a
// controller for the Content stream so a test decides when each chunk arrives.
function mockApi(previewHtml = '<p>önizleme</p>') {
  let stream!: ReadableStreamDefaultController<Uint8Array>
  const body = new ReadableStream<Uint8Array>({ start: (c) => void (stream = c) })
  const fetchMock = vi.fn(async (url: string, init?: RequestInit) => {
    const json = (b: unknown) => new Response(JSON.stringify(b), { headers: { 'Content-Type': 'application/json' } })
    if (url === '/api/campaigns') return json(campaign)
    if (url === '/api/campaigns/7/content') return new Response(body, { headers: { 'Content-Type': 'text/event-stream' } })
    if (url === '/api/email/preview') return new Response(previewHtml, { headers: { 'Content-Type': 'text/html' } })
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

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('Content review', () => {
  it('fills in the SMS and email Content as it streams', async () => {
    const api = mockApi()
    const wrapper = await submit()

    await api.send('content.delta', { channels: ['sms', 'email'], sms: { body: 'Merhaba {{first' } })
    expect(wrapper.find('[data-test="content-streaming"]').exists()).toBe(true)
    expect(wrapper.get('[data-test="sms-body"]').text()).toBe('Merhaba {{first')

    await api.send('content.delta', { channels: ['sms', 'email'], sms, email: { subject: '{{first_name}}, %30' } })
    expect(wrapper.get('[data-test="sms-body"]').text()).toBe(sms.body)
    expect(wrapper.get('[data-test="email-subject"]').text()).toBe('{{first_name}}, %30')

    await api.send('content.done', {
      enabled_channels: ['sms', 'email'],
      content: { sms, email },
      validation: { sms: valid, email: valid },
    })
    await api.end()

    expect(wrapper.find('[data-test="content-streaming"]').exists()).toBe(false)
    expect(wrapper.get('[data-test="email-subject"]').text()).toBe(email.subject)
    expect(wrapper.get('[data-test="email-preheader"]').text()).toBe(email.preheader)
    expect(wrapper.get('[data-test="email-cta"]').text()).toBe(email.cta_text)
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
