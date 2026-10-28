import { defineStore } from 'pinia'
import {
  ApiError,
  channels,
  createCampaign,
  previewEmail,
  setChannel,
  streamContent,
  updateContent,
  validateContent,
  type Campaign,
  type Channel,
  type Content,
  type RepairNotice,
  type Validation,
} from '../api'

const unreachable = 'Sunucuya ulaşılamadı. Lütfen tekrar deneyin.'

// validateDelay is how long typing must pause before the backend validator
// is asked about an edit.
export const validateDelay = 300

const message = (e: unknown) => (e instanceof ApiError ? e.message : unreachable)

export const useCampaignStore = defineStore('campaign', {
  state: () => ({
    campaign: null as Campaign | null,
    brief: '',
    generating: false,
    error: null as string | null,
    // retryable marks an error that another try may fix.
    retryable: false,
    content: {} as Content,
    enabledChannels: [] as Channel[],
    validation: {} as Partial<Record<Channel, Validation>>,
    streaming: false,
    contentError: null as string | null,
    contentRetryable: false,
    // repairing is set while Claude repairs rejected Content (ADR-0005).
    repairing: null as RepairNotice | null,
    emailPreview: null as string | null,
    // revisions count the Marketer's edits per Channel, so a slow validation
    // answer for older text never overwrites a newer one.
    revisions: { sms: 0, email: 0 } as Record<Channel, number>,
    validateTimers: {} as Partial<Record<Channel, ReturnType<typeof setTimeout>>>,
  }),
  actions: {
    async create(brief: string) {
      this.brief = brief
      this.generating = true
      this.error = null
      this.retryable = false
      this.campaign = null
      this.resetContent()
      try {
        this.campaign = await createCampaign(brief)
      } catch (e) {
        this.error = message(e)
        this.retryable = e instanceof ApiError ? e.retryable : true
      } finally {
        this.generating = false
      }
      if (this.campaign) await this.generateContent()
    },

    resetContent() {
      for (const c of channels) {
        clearTimeout(this.validateTimers[c])
        this.revisions[c]++
      }
      this.content = {}
      this.enabledChannels = []
      this.validation = {}
      this.contentError = null
      this.contentRetryable = false
      this.emailPreview = null
    },

    // retry runs the failed step again: the Segment call if no Campaign was
    // created, otherwise the Content call.
    async retry() {
      if (this.campaign) await this.generateContent()
      else if (this.brief) await this.create(this.brief)
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
            case 'content.repair':
              // The rejected text is dropped; the repaired attempt streams anew.
              this.resetContent()
              this.repairing = e.data
              break
            case 'content.done':
              this.repairing = null
              this.content = e.data.content
              this.enabledChannels = e.data.enabled_channels
              this.validation = e.data.validation
              break
            case 'error':
              // No invalid output is left on screen as a proposal.
              this.resetContent()
              this.contentError = e.data.error
              this.contentRetryable = e.data.retryable === true
              break
          }
        })
      } catch (e) {
        this.contentError = message(e)
        this.contentRetryable = true
      } finally {
        this.streaming = false
        this.repairing = null
      }
      if (this.content.email && !this.contentError) {
        try {
          this.emailPreview = await previewEmail(this.content.email)
        } catch {
          this.emailPreview = null
        }
      }
    },

    // edit applies the Marketer's change to one Channel's Content and asks the
    // backend validator about it once typing pauses, for the live counters.
    edit<C extends Channel>(channel: C, patch: Partial<NonNullable<Content[C]>>) {
      if (this.streaming) return
      this.content = { ...this.content, [channel]: { ...this.content[channel], ...patch } }
      const revision = ++this.revisions[channel]
      clearTimeout(this.validateTimers[channel])
      this.validateTimers[channel] = setTimeout(() => this.runValidation(channel, revision), validateDelay)
    },

    async runValidation(channel: Channel, revision: number) {
      const c = this.content[channel]
      if (!this.campaign || !c) return
      try {
        const v = await validateContent(this.campaign.id, channel, c)
        // A later keystroke's answer wins, whatever order the answers arrive in.
        if (revision === this.revisions[channel]) this.validation = { ...this.validation, [channel]: v }
      } catch (e) {
        this.contentError = message(e)
      }
    },

    // save stores the Marketer's edit of one Channel's Content.
    async save(channel: Channel) {
      const c = this.content[channel]
      if (!this.campaign || !c || this.streaming) return
      clearTimeout(this.validateTimers[channel])
      const revision = this.revisions[channel]
      try {
        const v = await updateContent(this.campaign.id, channel, c)
        if (revision === this.revisions[channel]) this.validation = { ...this.validation, [channel]: v }
      } catch (e) {
        this.contentError = message(e)
        return
      }
      if (channel === 'email' && this.content.email) {
        try {
          this.emailPreview = await previewEmail(this.content.email)
        } catch {
          // The previous preview stays.
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
