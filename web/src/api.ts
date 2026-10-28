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
