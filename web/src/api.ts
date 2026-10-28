export interface Overview {
  recipient_count: number
  llm: { mock: boolean; model: string }
}

export async function fetchOverview(): Promise<Overview> {
  const res = await fetch('/api/overview')
  if (!res.ok) throw new Error(`GET /api/overview: ${res.status}`)
  return res.json()
}
