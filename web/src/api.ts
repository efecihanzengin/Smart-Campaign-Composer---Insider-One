export interface Overview {
  recipient_count: number
  llm: { mock: boolean; model: string }
}

export async function fetchOverview(): Promise<Overview> {
  const res = await fetch('/api/overview')
  if (!res.ok) throw new Error(`GET /api/overview: ${res.status}`)
  return res.json()
}

export interface Audience {
  sms: number
  email: number
}

export interface Campaign {
  id: number
  status: string
  brief: string
  segment: {
    rule: unknown
    labels: string[]
    unsupported_criteria: string[]
  }
  audience: Audience
}

// ApiError carries the Marketer-facing message the API sent, if any.
export class ApiError extends Error {}

export async function createCampaign(brief: string): Promise<Campaign> {
  const res = await fetch('/api/campaigns', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ brief }),
  })
  const body = await res.json().catch(() => null)
  if (!res.ok) throw new ApiError(body?.error ?? `POST /api/campaigns: ${res.status}`)
  return body
}

export type Channel = 'sms' | 'email'
export const channels: Channel[] = ['sms', 'email']

export interface SmsContent {
  body: string
}

export interface EmailContent {
  subject: string
  preheader: string
  body: string
  cta_text?: string
}

export interface Content {
  sms?: SmsContent | null
  email?: EmailContent | null
}

// Length is how much of a field's limit Content uses once rendered with the
// longest placeholder values in the Audience; encoding is set for SMS only.
export interface Length {
  encoding?: 'gsm7' | 'ucs2'
  used: number
  limit: number
}

export interface Validation {
  valid: boolean
  issues: { field: string; message: string }[]
  usage?: Record<string, Length>
}

// ContentDelta is the Content parsed so far while it streams.
export interface ContentDelta extends Content {
  channels?: string[]
}

export interface ContentDone {
  enabled_channels: Channel[]
  content: Content
  validation: Partial<Record<Channel, Validation>>
}

export type ContentEvent =
  | { type: 'content.delta'; data: ContentDelta }
  | { type: 'content.done'; data: ContentDone }
  | { type: 'error'; data: { error: string } }

// streamContent runs the streamed Content call and hands every SSE event to
// onEvent as it arrives. It resolves when the stream ends.
export async function streamContent(id: number, onEvent: (e: ContentEvent) => void): Promise<void> {
  const res = await fetch(`/api/campaigns/${id}/content`, { method: 'POST' })
  if (!res.ok || !res.body) {
    const body = await res.json().catch(() => null)
    throw new ApiError(body?.error ?? `POST /api/campaigns/${id}/content: ${res.status}`)
  }
  const reader = res.body.pipeThrough(new TextDecoderStream()).getReader()
  let buffer = ''
  for (;;) {
    const { value, done } = await reader.read()
    if (done) break
    buffer += value
    let end: number
    while ((end = buffer.indexOf('\n\n')) >= 0) {
      const frame = buffer.slice(0, end)
      buffer = buffer.slice(end + 2)
      let type = ''
      let data = ''
      for (const line of frame.split('\n')) {
        if (line.startsWith('event: ')) type = line.slice(7)
        else if (line.startsWith('data: ')) data += line.slice(6)
      }
      if (type && data) onEvent({ type, data: JSON.parse(data) } as ContentEvent)
    }
  }
}

export async function setChannel(id: number, channel: Channel, enabled: boolean): Promise<Channel[]> {
  const res = await fetch(`/api/campaigns/${id}/channels/${channel}`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ enabled }),
  })
  const body = await res.json().catch(() => null)
  if (!res.ok) throw new ApiError(body?.error ?? `PUT channel: ${res.status}`)
  return body.enabled_channels
}

type ChannelContent = SmsContent | EmailContent

// validateContent runs the backend's Content validator over unsaved Content.
// The rules live only in the backend; the live counters come from here.
export async function validateContent(id: number, channel: Channel, content: ChannelContent): Promise<Validation> {
  const res = await fetch(`/api/campaigns/${id}/content/${channel}/validate`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(content),
  })
  const body = await res.json().catch(() => null)
  if (!res.ok) throw new ApiError(body?.error ?? `POST validate: ${res.status}`)
  return body
}

// updateContent saves the Marketer's edit and returns its validation.
export async function updateContent(id: number, channel: Channel, content: ChannelContent): Promise<Validation> {
  const res = await fetch(`/api/campaigns/${id}/content/${channel}`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(content),
  })
  const body = await res.json().catch(() => null)
  if (!res.ok) throw new ApiError(body?.error ?? `PUT content: ${res.status}`)
  return body.validation
}

// previewEmail renders email Content into the backend's fixed HTML template.
export async function previewEmail(email: EmailContent): Promise<string> {
  const res = await fetch('/api/email/preview', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(email),
  })
  if (!res.ok) throw new ApiError(`POST /api/email/preview: ${res.status}`)
  return res.text()
}
