import { flushPromises } from '@vue/test-utils'

// frame encodes one SSE event as the backend writes it.
export const frame = (event: string, data: unknown) => `event: ${event}\ndata: ${JSON.stringify(data)}\n\n`

// sse answers a request with a complete event stream.
export const sse = (...events: [string, unknown][]) =>
  new Response(events.map(([e, d]) => frame(e, d)).join(''), { headers: { 'Content-Type': 'text/event-stream' } })

// openStream answers a request with an event stream the test feeds event by
// event, so it can look at the screen in between.
export function openStream() {
  let controller!: ReadableStreamDefaultController<Uint8Array>
  const body = new ReadableStream<Uint8Array>({ start: (c) => void (controller = c) })
  const encoder = new TextEncoder()
  return {
    response: new Response(body, { headers: { 'Content-Type': 'text/event-stream' } }),
    send: async (event: string, data: unknown) => {
      controller.enqueue(encoder.encode(frame(event, data)))
      await flushPromises()
    },
    end: async () => {
      controller.close()
      await flushPromises()
    },
  }
}
