import { defineStore } from 'pinia'
import {
  ApiError,
  ApprovalRefused,
  approveCampaign,
  cancelCampaign,
  channels,
  createCampaign,
  previewEmail,
  regenerateContent,
  regenerateSegment,
  setChannel,
  streamContent,
  updateContent,
  validateContent,
  type Approval,
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

const channelNames: Record<Channel, string> = { sms: 'SMS', email: 'E-posta' }

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
    // segmentRepairing is set while Claude repairs a rejected Segment Rule.
    segmentRepairing: null as RepairNotice | null,
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
    cancelling: false,
    // regenerating is the Channel whose Content Claude is rewriting.
    regenerating: null as Channel | null,
    // lastRegeneration is the Channel regeneration a retry runs again.
    lastRegeneration: null as { channel: Channel; instruction: string } | null,
    segmentRegenerating: false,
    segmentError: null as string | null,
    segmentRetryable: false,
    lastSegmentInstruction: '',
    // revalidated is set once the Segment Rule changed under existing
    // Content, which was then validated again against the new Audience.
    revalidated: false,
  }),
  getters: {
    // disabledChannels explains each Channel turned off because its Audience
    // has no consenting Recipient.
    disabledChannels: (s): Partial<Record<Channel, string>> => s.campaign?.disabled_channels ?? {},
    // locked is true while the Content can't change: Claude is writing (the
    // Content or a new Segment Rule), or the Campaign is no longer a draft.
    locked: (s) => s.streaming || s.segmentRegenerating || (s.campaign !== null && s.campaign.status !== 'draft'),
    // approvalBlockers lists why the Campaign can't be approved yet. The
    // backend checks the same guards again when it freezes the Audience.
    approvalBlockers(s): string[] {
      if (s.streaming) return ['İçerik hâlâ üretiliyor; bitmesini bekleyin.']
      if (s.segmentRegenerating) return ['Hedef kitle yeniden üretiliyor; bitmesini bekleyin.']
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
      this.brief = brief
      this.generating = true
      this.error = null
      this.retryable = false
      this.campaign = null
      this.resetContent()
      try {
        this.campaign = await createCampaign(brief, (n) => (this.segmentRepairing = n))
      } catch (e) {
        this.error = message(e)
        this.retryable = e instanceof ApiError ? e.retryable : true
      } finally {
        this.generating = false
        this.segmentRepairing = null
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
      this.contentRetryable = false
      this.emailPreview = null
      this.unsaved = { sms: false, email: false }
      this.approval = null
      this.approvalError = null
      this.lastRegeneration = null
      this.revalidated = false
    },

    // retry runs the failed step again: a Channel's regeneration, the Segment
    // call if no Campaign was created, otherwise the Content call.
    async retry() {
      if (this.lastRegeneration) {
        const { channel, instruction } = this.lastRegeneration
        await this.regenerateChannel(channel, instruction)
      } else if (this.campaign) await this.generateContent()
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
                this.enabledChannels = channels.filter(
                  (c) => e.data.channels!.includes(c) && !this.disabledChannels[c],
                )
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

    // saveEdits stores every unsaved edit, so Claude and the validator see
    // the Content the Marketer sees.
    async saveEdits() {
      for (const c of channels) if (this.unsaved[c]) await this.save(c)
    },

    // regenerateChannel asks Claude to rewrite one Channel's Content with the
    // Marketer's optional instruction. Only that Channel streams and changes;
    // if Claude fails, its current Content stays.
    async regenerateChannel(channel: Channel, instruction: string) {
      if (!this.campaign || this.locked) return
      await this.saveEdits()
      const previous = { content: this.content[channel], validation: this.validation[channel] }
      const restore = () => {
        this.content = { ...this.content, [channel]: previous.content }
        this.validation = { ...this.validation, [channel]: previous.validation }
      }
      clearTimeout(this.validateTimers[channel])
      this.revisions[channel]++
      this.lastRegeneration = { channel, instruction }
      this.contentError = null
      this.contentRetryable = false
      this.streaming = true
      this.regenerating = channel
      try {
        await regenerateContent(this.campaign.id, channel, instruction, (e) => {
          switch (e.type) {
            case 'content.delta':
              this.content = { ...this.content, [channel]: e.data[channel] }
              break
            case 'content.repair':
              this.content = { ...this.content, [channel]: null }
              this.repairing = e.data
              break
            case 'content.done':
              this.repairing = null
              this.content = { ...this.content, [channel]: e.data.content[channel] }
              this.validation = { ...this.validation, [channel]: e.data.validation[channel] }
              this.enabledChannels = e.data.enabled_channels
              this.lastRegeneration = null
              break
            case 'error':
              restore()
              this.contentError = e.data.error
              this.contentRetryable = e.data.retryable === true
              break
          }
        })
      } catch (e) {
        restore()
        this.contentError = message(e)
        this.contentRetryable = true
      } finally {
        this.streaming = false
        this.regenerating = null
        this.repairing = null
      }
      if (channel === 'email' && this.content.email && !this.contentError) {
        try {
          this.emailPreview = await previewEmail(this.content.email)
        } catch {
          // The previous preview stays.
        }
      }
    },

    // regenerateSegment asks Claude for a new Segment Rule. The Content is
    // kept, edits included, and only validated again against the new Audience.
    async regenerateSegment(instruction: string) {
      if (!this.campaign || this.locked) return
      await this.saveEdits()
      this.segmentRegenerating = true
      this.segmentError = null
      this.segmentRetryable = false
      this.lastSegmentInstruction = instruction
      try {
        const r = await regenerateSegment(this.campaign.id, instruction, (n) => (this.segmentRepairing = n))
        const { id, status, brief, segment, audience, repairs, enabled_channels, disabled_channels } = r
        this.campaign = { id, status, brief, segment, audience, repairs, enabled_channels, disabled_channels }
        // Validation answers for the old Audience must not overwrite these.
        for (const c of channels) this.revisions[c]++
        this.validation = r.validation
        this.enabledChannels = r.enabled_channels
        this.revalidated = true
      } catch (e) {
        this.segmentError = message(e)
        this.segmentRetryable = e instanceof ApiError ? e.retryable : true
      } finally {
        this.segmentRegenerating = false
        this.segmentRepairing = null
      }
    },

    async retrySegment() {
      await this.regenerateSegment(this.lastSegmentInstruction)
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

    // cancelDraft cancels the draft Campaign; it can then no longer be edited
    // or approved. An approved or sending Campaign is cancelled from the dashboard.
    async cancelDraft() {
      if (!this.campaign || this.campaign.status !== 'draft' || this.cancelling) return
      this.cancelling = true
      this.approvalError = null
      try {
        const status = await cancelCampaign(this.campaign.id)
        this.campaign = { ...this.campaign, status }
      } catch (e) {
        this.approvalError = message(e)
      } finally {
        this.cancelling = false
      }
    },
  },
})
