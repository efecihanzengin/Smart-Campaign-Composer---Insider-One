import { afterEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia } from 'pinia'
import CampaignComposer from './CampaignComposer.vue'

const exampleBrief =
  "Son 30 günde sepetine ürün ekleyip alışveriş yapmayan İstanbul'daki kullanıcılara %30 indirim hatırlatması gönder. Samimi ama kısa olsun."

function campaign(overrides: { unsupported_criteria?: string[] } = {}) {
  return {
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

function mockCreate(body: unknown, status = 201) {
  const fetchMock = vi.fn().mockResolvedValue(
    new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } }),
  )
  vi.stubGlobal('fetch', fetchMock)
  return fetchMock
}

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
    expect(JSON.parse(fetchMock.mock.calls[0][1].body)).toEqual({ brief: exampleBrief })
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

  it('shows the API error when generation fails', async () => {
    mockCreate({ error: 'Model geçersiz bir hedef kitle kuralı üretti. Lütfen tekrar deneyin.' }, 502)

    const wrapper = await submitBrief()

    expect(wrapper.get('[data-test="campaign-error"]').text()).toContain('geçersiz bir hedef kitle kuralı')
    expect(wrapper.find('[data-test="segment-label"]').exists()).toBe(false)
  })

  it('offers a retry when the Segment Rule could not be repaired', async () => {
    const fetchMock = mockCreate(
      { error: 'Model geçerli bir hedef kitle kuralı üretemedi. Lütfen tekrar deneyin.', retryable: true },
      502,
    )
    const wrapper = await submitBrief()
    expect(wrapper.find('[data-test="segment-label"]').exists()).toBe(false)

    fetchMock.mockResolvedValue(
      new Response(JSON.stringify(campaign()), { status: 201, headers: { 'Content-Type': 'application/json' } }),
    )
    await wrapper.get('[data-test="campaign-retry"]').trigger('click')
    await flushPromises()

    const creates = fetchMock.mock.calls.filter(([url]) => url === '/api/campaigns')
    expect(creates).toHaveLength(2)
    expect(JSON.parse(creates[1][1].body)).toEqual({ brief: exampleBrief })
    expect(wrapper.find('[data-test="campaign-error"]').exists()).toBe(false)
    expect(wrapper.findAll('[data-test="segment-label"]')).toHaveLength(3)
  })

  it('offers no retry for an error that is not retryable', async () => {
    mockCreate({ error: 'Kampanya talebi boş olamaz.' }, 400)

    const wrapper = await submitBrief()

    expect(wrapper.get('[data-test="campaign-error"]').text()).toContain('boş olamaz')
    expect(wrapper.find('[data-test="campaign-retry"]').exists()).toBe(false)
  })
})
