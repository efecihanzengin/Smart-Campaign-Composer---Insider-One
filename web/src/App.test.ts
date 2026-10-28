import { afterEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia } from 'pinia'
import App from './App.vue'

function mockOverview(body: unknown, status = 200) {
  const fetchMock = vi.fn().mockResolvedValue(
    new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } }),
  )
  vi.stubGlobal('fetch', fetchMock)
  return fetchMock
}

async function mountApp() {
  const wrapper = mount(App, { global: { plugins: [createPinia()] } })
  await flushPromises()
  return wrapper
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('App', () => {
  it('shows the Recipient count from the API', async () => {
    const fetchMock = mockOverview({ recipient_count: 5000, llm: { mock: true, model: 'claude-sonnet-5-5' } })

    const wrapper = await mountApp()

    expect(fetchMock).toHaveBeenCalledWith('/api/overview')
    expect(wrapper.get('[data-test="recipient-count"]').text()).toBe('5.000')
  })

  it('shows the Mock LLM badge when the API runs on the mock LLM', async () => {
    mockOverview({ recipient_count: 5000, llm: { mock: true, model: 'claude-sonnet-5-5' } })

    const wrapper = await mountApp()

    expect(wrapper.get('[data-test="mock-llm-badge"]').text()).toBe('Mock LLM')
  })

  it('hides the Mock LLM badge when the real model is configured', async () => {
    mockOverview({ recipient_count: 5000, llm: { mock: false, model: 'claude-sonnet-5-5' } })

    const wrapper = await mountApp()

    expect(wrapper.find('[data-test="mock-llm-badge"]').exists()).toBe(false)
  })

  it('shows an error when the API is unreachable', async () => {
    mockOverview({ error: 'internal error' }, 500)

    const wrapper = await mountApp()

    expect(wrapper.get('[data-test="overview-error"]').text()).toContain('Sunucuya ulaşılamadı')
  })
})
