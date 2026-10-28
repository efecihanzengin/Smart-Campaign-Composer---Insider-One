import { defineStore } from 'pinia'
import {
  ApiError,
  ApprovalRefused,
  approveCampaign,
  channels,
  createCampaign,
  previewEmail,
  setChannel,
  streamContent,
  updateContent,
  validateContent,
  type Approval,
  type Campaign,
  type Channel,
  type Content,
  type Validation,
} from '../api'

const unreachable = 'Sunucuya ulaşılamadı. Lütfen tekrar deneyin.'

// validateDelay is how long typing must pause before the backend validator
// is asked about an edit.
export const validateDelay = 300

const message = (e: unknown) => (e instanceof ApiError ? e.message : unreachable)

const channelNames: Record<Channel, string> = { sms: 'SMS', email: 'E-posta' }

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
    // revisions count the Marketer's edits per Channel, so a slow validation
    // answer for older text never overwrites a newer one.
    revisions: { sms: 0, email: 0 } as Record<Channel, number>,
    validateTimers: {} as Partial<Record<Channel, ReturnType<typeof setTimeout>>>,
    // unsaved marks Channels edited since their last save.
    unsaved: { sms: false, email: false } as Record<Channel, boolean>,
    approving: false,
    approval: null as Approval | null,
    approvalError: null as string | null,
  }),
  getters: {
    // disabledChannels explains each Channel turned off because its Audience
    // has no consenting Recipient.
    disabledChannels: (s): Partial<Record<Channel, string>> => s.campaign?.disabled_channels ?? {},
    // locked is true while the Content can't change: it is streaming, or the
    // Campaign is no longer a draft.
    locked: (s) => s.streaming || (s.campaign !== null && s.campaign.status !== 'draft'),
    // approvalBlockers lists why the Campaign can't be approved yet. The
    // backend checks the same guards again when it freezes the Audience.
    approvalBlockers(s): string[] {
      if (s.streaming) return ['İçerik hâlâ üretiliyor; bitmesini bekleyin.']
      if (!s.campaign) return []
      if (s.enabledChannels.length === 0) return ['En az bir kanal açık olmalı.']
      const out: string[] = []
      for (const c of s.enabledChannels) {
        if (!s.content[c]) out.push(`${channelNames[c]} içeriği yok.`)
        else if (!s.validation[c]?.valid) out.push(`${channelNames[c]} içeriği kurallara uymuyor.`)
        if (s.campaign.audience[c] === 0) out.push(`${channelNames[c]} kanalında izin vermiş alıcı yok.`)
      }
      return out
    },
  },
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
      if (this.campaign) {
        this.enabledChannels = this.campaign.enabled_channels ?? [...channels]
        await this.generateContent()
      }
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
      this.emailPreview = null
      this.unsaved = { sms: false, email: false }
      this.approval = null
      this.approvalError = null
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
                this.enabledChannels = channels.filter(
                  (c) => e.data.channels!.includes(c) && !this.disabledChannels[c],
                )
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

    // edit applies the Marketer's change to one Channel's Content and asks the
    // backend validator about it once typing pauses, for the live counters.
    edit<C extends Channel>(channel: C, patch: Partial<NonNullable<Content[C]>>) {
      if (this.locked) return
      this.content = { ...this.content, [channel]: { ...this.content[channel], ...patch } }
      this.unsaved[channel] = true
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
      if (!this.campaign || !c || this.locked) return
      clearTimeout(this.validateTimers[channel])
      const revision = this.revisions[channel]
      try {
        const v = await updateContent(this.campaign.id, channel, c)
        if (revision === this.revisions[channel]) this.unsaved[channel] = false
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
      if (!this.campaign || this.locked) return
      try {
        this.enabledChannels = await setChannel(this.campaign.id, channel, enabled)
      } catch (e) {
        this.contentError = message(e)
      }
    },

    // approve freezes the Audience into Deliveries, saving unsaved edits first
    // so the backend approves exactly what the Marketer sees.
    async approve() {
      if (!this.campaign || this.approving || this.approvalBlockers.length) return
      this.approving = true
      this.approvalError = null
      try {
        for (const c of channels) {
          if (this.unsaved[c]) await this.save(c)
        }
        if (this.contentError || channels.some((c) => this.unsaved[c])) return
        if (this.approvalBlockers.length) return
        this.approval = await approveCampaign(this.campaign.id)
        this.campaign = { ...this.campaign, status: this.approval.status }
      } catch (e) {
        this.approvalError =
          e instanceof ApprovalRefused && e.reasons.length ? e.reasons.map((r) => r.message).join(' ') : message(e)
      } finally {
        this.approving = false
      }
    },
  },
})
