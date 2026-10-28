import { defineStore } from 'pinia'
import {
  ApiError,
  channels,
  createCampaign,
  previewEmail,
  setChannel,
  streamContent,
  type Campaign,
  type Channel,
  type Content,
  type Validation,
} from '../api'

const unreachable = 'Sunucuya ulaşılamadı. Lütfen tekrar deneyin.'

const message = (e: unknown) => (e instanceof ApiError ? e.message : unreachable)

export const useCampaignStore = defineStore('campaign', {
  state: () => ({
    campaign: null as Campaign | null,
    generating: false,
    error: null as string | null,
    content: {} as Content,
    enabledChannels: [] as Channel[],
    validation: {} as Partial<Record<Channel, Validation>>,
    streaming: false,
    contentError: null as string | null,
    emailPreview: null as string | null,
  }),
  actions: {
    async create(brief: string) {
      this.generating = true
      this.error = null
      this.campaign = null
      this.resetContent()
      try {
        this.campaign = await createCampaign(brief)
      } catch (e) {
        this.error = message(e)
      } finally {
        this.generating = false
      }
      if (this.campaign) await this.generateContent()
    },

    resetContent() {
      this.content = {}
      this.enabledChannels = []
      this.validation = {}
      this.contentError = null
      this.emailPreview = null
    },

    // generateContent streams the Content onto the screen as Claude writes it.
    async generateContent() {
      if (!this.campaign) return
      this.resetContent()
      this.streaming = true
      try {
        await streamContent(this.campaign.id, (e) => {
          switch (e.type) {
            case 'content.delta':
              this.content = { sms: e.data.sms, email: e.data.email }
              if (e.data.channels) {
                this.enabledChannels = channels.filter((c) => e.data.channels!.includes(c))
              }
              break
            case 'content.done':
              this.content = e.data.content
              this.enabledChannels = e.data.enabled_channels
              this.validation = e.data.validation
              break
            case 'error':
              this.contentError = e.data.error
              break
          }
        })
      } catch (e) {
        this.contentError = message(e)
      } finally {
        this.streaming = false
      }
      if (this.content.email && !this.contentError) {
        try {
          this.emailPreview = await previewEmail(this.content.email)
        } catch {
          this.emailPreview = null
        }
      }
    },

    async toggleChannel(channel: Channel, enabled: boolean) {
      if (!this.campaign) return
      try {
        this.enabledChannels = await setChannel(this.campaign.id, channel, enabled)
      } catch (e) {
        this.contentError = message(e)
      }
    },
  },
})
