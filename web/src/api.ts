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
  // Channels with an empty Audience start disabled; disabled_channels says why.
  enabled_channels?: Channel[]
  disabled_channels?: Partial<Record<Channel, string>>
  // repairs is how many repairs the Segment call needed (ADR-0005).
  repairs?: number
  // mock_brief_unrecognised is set when the mock LLM answered a Brief other
  // than the example Brief, so its proposal is only a guess.
  mock_brief_unrecognised?: boolean
}

// ApiError carries the Marketer-facing message the API sent, if any, and
// whether trying again may help (Claude's output could not be repaired).
export class ApiError extends Error {
  constructor(
    message: string,
    readonly retryable = false,
  ) {
    super(message)
  }
}

// createCampaign creates a Campaign from a Brief. Claude's Segment Rule
// arrives over SSE; onRepair hears about each repair of a rejected rule while
// it runs (ADR-0005).
export function createCampaign(brief: string, onRepair: (n: RepairNotice) => void): Promise<Campaign> {
  return streamSegment<Campaign>(
    '/api/campaigns',
    { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ brief }) },
    onRepair,
  )
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
  repairs?: number
}

// RepairNotice says Claude's output was rejected and is being repaired.
export interface RepairNotice {
  repair: number
  max: number
  errors: string[]
}

interface StreamError {
  type: 'error'
  data: { error: string; retryable?: boolean }
}

export type ContentEvent =
  | { type: 'content.delta'; data: ContentDelta }
  | { type: 'content.repair'; data: RepairNotice }
  | { type: 'content.done'; data: ContentDone }
  | StreamError

type SegmentEvent<T> = { type: 'segment.repair'; data: RepairNotice } | { type: 'segment.done'; data: T } | StreamError

// streamSegment runs a streamed Segment call and resolves to its result, or
// rejects with the error the stream ended with.
async function streamSegment<T>(url: string, init: RequestInit, onRepair: (n: RepairNotice) => void): Promise<T> {
  let result: T | undefined
  let failure: ApiError | undefined
  await readStream<SegmentEvent<T>>(url, init, (e) => {
    if (e.type === 'segment.repair') onRepair(e.data)
    else if (e.type === 'segment.done') result = e.data
    else failure = new ApiError(e.data.error, e.data.retryable === true)
  })
  if (failure) throw failure
  if (result === undefined) throw new ApiError('Sunucuya ulaşılamadı. Lütfen tekrar deneyin.', true)
  return result
}

// streamContent runs the streamed Content call and hands every SSE event to
// onEvent as it arrives. It resolves when the stream ends.
export function streamContent(id: number, onEvent: (e: ContentEvent) => void): Promise<void> {
  return readStream(`/api/campaigns/${id}/content`, { method: 'POST' }, onEvent)
}

// regenerateContent asks Claude to rewrite one Channel's Content, with the
// Marketer's optional instruction. Deltas carry only that Channel; done
// carries the Campaign's whole stored Content.
export function regenerateContent(
  id: number,
  channel: Channel,
  instruction: string,
  onEvent: (e: ContentEvent) => void,
): Promise<void> {
  return readStream(
    `/api/campaigns/${id}/content/${channel}/regenerate`,
    { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ instruction }) },
    onEvent,
  )
}

// RegeneratedSegment is the Campaign with its new Segment Rule and Audience,
// and its kept Content validated against the new Audience.
export interface RegeneratedSegment extends Campaign {
  enabled_channels: Channel[]
  content: Content
  validation: Partial<Record<Channel, Validation>>
}

// regenerateSegment asks Claude for a new Segment Rule, streamed like the
// first one. Content is never regenerated with it, only validated again.
export function regenerateSegment(
  id: number,
  instruction: string,
  onRepair: (n: RepairNotice) => void,
): Promise<RegeneratedSegment> {
  return streamSegment<RegeneratedSegment>(
    `/api/campaigns/${id}/segment/regenerate`,
    { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ instruction }) },
    onRepair,
  )
}

// readStream POSTs a generation request and hands every SSE event to onEvent
// as it arrives. It resolves when the stream ends; a request refused before
// the stream opened rejects with the API's message.
async function readStream<E>(url: string, init: RequestInit, onEvent: (e: E) => void): Promise<void> {
  const res = await fetch(url, init)
  if (!res.ok || !res.body) {
    const body = await res.json().catch(() => null)
    throw new ApiError(body?.error ?? `POST ${url}: ${res.status}`, body?.retryable === true)
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
      if (type && data) onEvent({ type, data: JSON.parse(data) } as E)
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

// ApprovalReason is one reason the backend refused approval.
export interface ApprovalReason {
  code: string
  channel?: Channel
  message: string
}

export interface Approval {
  id: number
  status: string
  deliveries: Partial<Record<Channel, number>>
}

// ApprovalRefused carries the reasons the backend refused approval for.
export class ApprovalRefused extends ApiError {
  constructor(
    message: string,
    readonly reasons: ApprovalReason[],
  ) {
    super(message)
  }
}

// approveCampaign freezes the Campaign's Audience into Deliveries.
export async function approveCampaign(id: number): Promise<Approval> {
  const res = await fetch(`/api/campaigns/${id}/approve`, { method: 'POST' })
  const body = await res.json().catch(() => null)
  if (res.status === 422) throw new ApprovalRefused(body?.error ?? 'Kampanya onaylanamaz.', body?.reasons ?? [])
  if (!res.ok) throw new ApiError(body?.error ?? `POST approve: ${res.status}`)
  return body
}

// cancellable lists the statuses a Campaign can be cancelled from.
export const cancellable = ['draft', 'approved', 'sending']

// cancelCampaign stops a Campaign: Deliveries not yet sent are cancelled, and
// those already in flight finish. It resolves to the new status.
export async function cancelCampaign(id: number): Promise<string> {
  const res = await fetch(`/api/campaigns/${id}/cancel`, { method: 'POST' })
  const body = await res.json().catch(() => null)
  if (!res.ok) throw new ApiError(body?.error ?? `POST cancel: ${res.status}`)
  return body.status
}

// ChannelCounts is a Channel's Deliveries by state; pending includes those in
// flight, and cancelled counts those a cancelled Campaign never sent.
export interface ChannelCounts {
  pending: number
  sent: number
  failed: number
  cancelled?: number
  total: number
}

// CampaignProgress is one Campaign on the live dashboard.
export interface CampaignProgress {
  id: number
  brief: string
  status: string
  deliveries: Partial<Record<Channel, ChannelCounts>>
  // progress is the share of Deliveries with an outcome (sent, failed or
  // cancelled), from 0 to 1.
  progress: number
}

export interface DashboardEvent {
  campaigns: CampaignProgress[]
}

// subscribeDashboard opens the dashboard SSE stream and hands every event to
// onEvent. EventSource reconnects on its own; the returned function closes it.
export function subscribeDashboard(onEvent: (e: DashboardEvent) => void): () => void {
  const source = new EventSource('/api/dashboard')
  source.addEventListener('dashboard', (e) => onEvent(JSON.parse((e as MessageEvent).data)))
  return () => source.close()
}
