import { afterEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia } from 'pinia'
import CampaignComposer from './CampaignComposer.vue'
import { openStream, sse } from '../testing/sse'

const exampleBrief =
  "Son 30 günde sepetine ürün ekleyip alışveriş yapmayan İstanbul'daki kullanıcılara %30 indirim hatırlatması gönder. Samimi ama kısa olsun."

function campaign(overrides: { unsupported_criteria?: string[]; mock_brief_unrecognised?: boolean } = {}) {
  return {
    mock_brief_unrecognised: overrides.mock_brief_unrecognised ?? false,
    id: 1,
    status: 'draft',
    brief: exampleBrief,
    segment: {
      rule: { all: [] },
      labels: ['Şehir: İstanbul', 'Son 30 günde sepete ekledi', 'Son 30 günde satın almadı'],
      unsupported_criteria: overrides.unsupported_criteria ?? [],
    },
    audience: { sms: 412, email: 1534 },
  }
}

// mockFetch answers the Segment call with response (a fresh one per call).
function mockFetch(response: () => Response) {
  const fetchMock = vi.fn(async () => response())
  vi.stubGlobal('fetch', fetchMock)
  return fetchMock
}

// mockCreate streams the created Campaign as the Segment call's result.
const mockCreate = (body: unknown) => mockFetch(() => sse(['segment.done', body]))

const failedCall = { error: 'Model geçerli bir hedef kitle kuralı üretemedi. Lütfen tekrar deneyin.', retryable: true }

async function submitBrief(brief = exampleBrief) {
  const wrapper = mount(CampaignComposer, { global: { plugins: [createPinia()] } })
  await wrapper.get('[data-test="brief"]').setValue(brief)
  await wrapper.get('[data-test="brief-form"]').trigger('submit')
  await flushPromises()
  return wrapper
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('CampaignComposer', () => {
  it('shows the example Brief as the placeholder', () => {
    const wrapper = mount(CampaignComposer, { global: { plugins: [createPinia()] } })

    expect(wrapper.get('[data-test="brief"]').attributes('placeholder')).toBe(exampleBrief)
  })

  it('cannot be submitted with an empty Brief', async () => {
    const wrapper = mount(CampaignComposer, { global: { plugins: [createPinia()] } })

    await wrapper.get('[data-test="brief"]').setValue('   ')

    expect(wrapper.get('[data-test="submit"]').attributes('disabled')).toBeDefined()
  })

  it('sends the Brief and shows the Segment Rule labels', async () => {
    const fetchMock = mockCreate(campaign())

    const wrapper = await submitBrief()

    expect(fetchMock).toHaveBeenCalledWith('/api/campaigns', expect.objectContaining({ method: 'POST' }))
    const [, init] = fetchMock.mock.calls[0] as unknown as [string, RequestInit]
    expect(JSON.parse(String(init.body))).toEqual({ brief: exampleBrief })
    expect(wrapper.findAll('[data-test="segment-label"]').map((l) => l.text())).toEqual([
      'Şehir: İstanbul',
      'Son 30 günde sepete ekledi',
      'Son 30 günde satın almadı',
    ])
  })

  it('shows the Audience count per Channel', async () => {
    mockCreate(campaign())

    const wrapper = await submitBrief()

    expect(wrapper.get('[data-test="audience-sms"]').text()).toBe('412')
    expect(wrapper.get('[data-test="audience-email"]').text()).toBe('1.534')
  })

  it('warns about unsupported criteria above the labels', async () => {
    mockCreate(campaign({ unsupported_criteria: ['ayakkabı satın alanlar'] }))

    const wrapper = await submitBrief()

    const warning = wrapper.get('[data-test="unsupported-warning"]')
    expect(warning.text()).toContain('ayakkabı satın alanlar')
    const html = wrapper.html()
    expect(html.indexOf('data-test="unsupported-warning"')).toBeLessThan(html.indexOf('data-test="segment-label"'))
  })

  it('shows no warning when every criterion is supported', async () => {
    mockCreate(campaign())

    const wrapper = await submitBrief()

    expect(wrapper.find('[data-test="unsupported-warning"]').exists()).toBe(false)
  })

  it('tells the Marketer when the mock LLM did not recognise the Brief', async () => {
    mockCreate(campaign({ mock_brief_unrecognised: true }))

    const wrapper = await submitBrief('Tüm kullanıcılara yeni yıl kutlaması gönder.')

    expect(wrapper.get('[data-test="mock-brief-notice"]').text()).toBe(
      "Mock LLM yalnızca örnek brief'i tanır; gerçek sonuç için ANTHROPIC_API_KEY girin.",
    )
  })

  it('shows no mock notice for the example Brief', async () => {
    mockCreate(campaign())

    const wrapper = await submitBrief()

    expect(wrapper.find('[data-test="mock-brief-notice"]').exists()).toBe(false)
  })

  it('shows the API error when generation fails', async () => {
    mockFetch(() => sse(['error', { error: 'Kampanya kaydedilemedi. Lütfen tekrar deneyin.' }]))

    const wrapper = await submitBrief()

    expect(wrapper.get('[data-test="campaign-error"]').text()).toContain('Kampanya kaydedilemedi')
    expect(wrapper.find('[data-test="segment-label"]').exists()).toBe(false)
  })

  it('shows that the Segment Rule is being repaired until the repaired rule arrives', async () => {
    const stream = openStream()
    mockFetch(() => stream.response)
    const wrapper = await submitBrief()

    await stream.send('segment.repair', { repair: 1, max: 2, errors: ['rule.all[0].field: unknown field'] })
    const indicator = wrapper.get('[data-test="segment-repairing"]')
    expect(indicator.text()).toContain('1/2')
    expect(wrapper.get('[data-test="submit"]').attributes('disabled')).toBeDefined()

    await stream.send('segment.done', campaign())
    await stream.end()
    expect(wrapper.find('[data-test="segment-repairing"]').exists()).toBe(false)
    expect(wrapper.findAll('[data-test="segment-label"]')).toHaveLength(3)
  })

  it('offers a retry when the Segment Rule could not be repaired', async () => {
    let answer = () =>
      sse(['segment.repair', { repair: 1, max: 2, errors: ['x'] }], ['segment.repair', { repair: 2, max: 2, errors: ['x'] }], [
        'error',
        failedCall,
      ])
    const fetchMock = mockFetch(() => answer())
    const wrapper = await submitBrief()
    expect(wrapper.find('[data-test="segment-label"]').exists()).toBe(false)
    expect(wrapper.find('[data-test="segment-repairing"]').exists()).toBe(false)
    expect(wrapper.get('[data-test="campaign-error"]').text()).toContain('üretemedi')

    answer = () => sse(['segment.done', campaign()])
    await wrapper.get('[data-test="campaign-retry"]').trigger('click')
    await flushPromises()

    const creates = (fetchMock.mock.calls as unknown as [string, RequestInit][]).filter(([url]) => url === '/api/campaigns')
    expect(creates).toHaveLength(2)
    expect(JSON.parse(String(creates[1][1].body))).toEqual({ brief: exampleBrief })
    expect(wrapper.find('[data-test="campaign-error"]').exists()).toBe(false)
    expect(wrapper.findAll('[data-test="segment-label"]')).toHaveLength(3)
  })

  it('offers no retry for an error that is not retryable', async () => {
    mockFetch(
      () =>
        new Response(JSON.stringify({ error: 'Kampanya talebi boş olamaz.' }), {
          status: 400,
          headers: { 'Content-Type': 'application/json' },
        }),
    )

    const wrapper = await submitBrief()

    expect(wrapper.get('[data-test="campaign-error"]').text()).toContain('boş olamaz')
    expect(wrapper.find('[data-test="campaign-retry"]').exists()).toBe(false)
  })
})
