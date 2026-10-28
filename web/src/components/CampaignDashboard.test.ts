import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { mount, type VueWrapper } from '@vue/test-utils'
import { nextTick } from 'vue'
import CampaignDashboard from './CampaignDashboard.vue'

// FakeEventSource stands in for the browser's EventSource, so the test can
// push dashboard events as the backend would.
class FakeEventSource {
  static last: FakeEventSource | null = null
  listeners: Record<string, ((e: MessageEvent) => void)[]> = {}
  closed = false
  constructor(readonly url: string) {
    FakeEventSource.last = this
  }
  addEventListener(type: string, fn: (e: MessageEvent) => void) {
    ;(this.listeners[type] ??= []).push(fn)
  }
  close() {
    this.closed = true
  }
  async emit(type: string, data: unknown) {
    for (const fn of this.listeners[type] ?? []) fn(new MessageEvent(type, { data: JSON.stringify(data) }))
    await nextTick()
  }
}

const campaign = (over: Record<string, unknown> = {}) => ({
  id: 7,
  brief: 'Sepet hatırlatması gönder',
  status: 'sending',
  deliveries: {
    sms: { pending: 300, sent: 112, failed: 0, total: 412 },
    email: { pending: 1000, sent: 534, failed: 0, total: 1534 },
  },
  progress: 646 / 1946,
  ...over,
})

const source = () => FakeEventSource.last!
const row = (w: VueWrapper, id: number) => w.get(`[data-test="dashboard-campaign-${id}"]`)
const counts = (w: VueWrapper, id: number, channel: string) =>
  row(w, id).get(`[data-test="counts-${channel}"]`).text().replace(/\s+/g, ' ')
const bar = (w: VueWrapper, id: number) => row(w, id).get('[data-test="progress"]')

beforeEach(() => {
  FakeEventSource.last = null
  vi.stubGlobal('EventSource', FakeEventSource)
})

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('Campaign dashboard', () => {
  it('subscribes to the dashboard stream', () => {
    mount(CampaignDashboard)

    expect(source().url).toBe('/api/dashboard')
  })

  it('shows pending, sent and failed counts per Channel and a progress bar', async () => {
    const wrapper = mount(CampaignDashboard)
    await source().emit('dashboard', { campaigns: [campaign()] })

    expect(counts(wrapper, 7, 'sms')).toContain('Bekleyen 300')
    expect(counts(wrapper, 7, 'sms')).toContain('Gönderildi 112')
    expect(counts(wrapper, 7, 'sms')).toContain('Başarısız 0')
    expect(counts(wrapper, 7, 'email')).toContain('Bekleyen 1.000')
    expect(counts(wrapper, 7, 'email')).toContain('Gönderildi 534')
    expect(bar(wrapper, 7).attributes('aria-valuenow')).toBe('33')
    expect(row(wrapper, 7).text()).toContain('Gönderiliyor')
  })

  it('updates live as new events arrive, without a refresh', async () => {
    const wrapper = mount(CampaignDashboard)
    await source().emit('dashboard', { campaigns: [campaign()] })

    await source().emit('dashboard', {
      campaigns: [
        campaign({
          status: 'completed',
          deliveries: {
            sms: { pending: 0, sent: 412, failed: 0, total: 412 },
            email: { pending: 0, sent: 1534, failed: 0, total: 1534 },
          },
          progress: 1,
        }),
      ],
    })

    expect(counts(wrapper, 7, 'sms')).toContain('Gönderildi 412')
    expect(counts(wrapper, 7, 'sms')).toContain('Bekleyen 0')
    expect(bar(wrapper, 7).attributes('aria-valuenow')).toBe('100')
    expect(row(wrapper, 7).text()).toContain('Tamamlandı')
  })

  it('lists only the Channels a Campaign sends on', async () => {
    const wrapper = mount(CampaignDashboard)
    await source().emit('dashboard', {
      campaigns: [campaign({ deliveries: { email: { pending: 5, sent: 0, failed: 0, total: 5 } }, progress: 0 })],
    })

    expect(row(wrapper, 7).find('[data-test="counts-sms"]').exists()).toBe(false)
    expect(counts(wrapper, 7, 'email')).toContain('Bekleyen 5')
  })

  it('shows every Campaign in the stream', async () => {
    const wrapper = mount(CampaignDashboard)
    await source().emit('dashboard', { campaigns: [campaign({ id: 8, status: 'approved' }), campaign()] })

    expect(row(wrapper, 8).text()).toContain('Onaylandı')
    expect(wrapper.find('[data-test="dashboard-campaign-7"]').exists()).toBe(true)
  })

  it('stays hidden until a Campaign has been approved', async () => {
    const wrapper = mount(CampaignDashboard)
    await source().emit('dashboard', { campaigns: [] })

    expect(wrapper.find('[data-test="dashboard"]').exists()).toBe(false)
  })

  it('closes the stream when it is unmounted', () => {
    const wrapper = mount(CampaignDashboard)
    wrapper.unmount()

    expect(source().closed).toBe(true)
  })
})
