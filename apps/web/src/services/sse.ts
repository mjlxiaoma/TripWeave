import { getAccessToken, refreshSession } from './api'

const BASE: string = import.meta.env.VITE_API_BASE ?? '/api/v1'

export interface SSEEvent {
  event: string
  data: string
}

// parseSSEStream incrementally parses an SSE byte stream, invoking onEvent for
// each complete "event:/data:" block. Pure and synchronous per chunk so it is
// easy to unit test.
export function parseSSEStream(
  onEvent: (e: SSEEvent) => void,
): (chunk: string) => void {
  let buffer = ''
  return (chunk: string) => {
    buffer += chunk
    // Events are separated by a blank line.
    let idx: number
    while ((idx = buffer.indexOf('\n\n')) !== -1) {
      const block = buffer.slice(0, idx)
      buffer = buffer.slice(idx + 2)
      let event = 'message'
      const dataLines: string[] = []
      for (const line of block.split('\n')) {
        if (line.startsWith(':')) continue // comment / keep-alive ping
        if (line.startsWith('event:')) event = line.slice(6).trim()
        else if (line.startsWith('data:')) dataLines.push(line.slice(5).trimStart())
      }
      if (dataLines.length > 0) {
        onEvent({ event, data: dataLines.join('\n') })
      }
    }
  }
}

export interface StreamHandlers {
  onEvent: (e: SSEEvent) => void
  onError?: (err: Error) => void
}

// streamSSE POSTs and reads the response as an SSE stream. EventSource cannot
// send Authorization headers or POST, so we use fetch + getReader. On 401 it
// refreshes the session once and retries.
export async function streamSSE(
  path: string,
  body: unknown,
  handlers: StreamHandlers,
  signal?: AbortSignal,
  retry = true,
): Promise<void> {
  const token = getAccessToken()
  let res: Response
  try {
    res = await fetch(`${BASE}${path}`, {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Accept: 'text/event-stream',
        ...(token ? { Authorization: `Bearer ${token}` } : {}),
      },
      body: JSON.stringify(body),
      signal,
    })
  } catch (err) {
    if ((err as Error).name === 'AbortError') return
    handlers.onError?.(err as Error)
    return
  }

  if (res.status === 401 && retry) {
    if (await refreshSession()) {
      return streamSSE(path, body, handlers, signal, false)
    }
  }
  if (!res.ok || !res.body) {
    // Non-streaming error envelope (e.g. AI_BUSY, TRIP_NOT_FOUND).
    let code = 'UNKNOWN'
    let message = res.statusText
    try {
      const env = (await res.json()) as { error?: { code?: string; message?: string } }
      code = env.error?.code ?? code
      message = env.error?.message ?? message
    } catch {
      /* not JSON */
    }
    const err = new Error(message)
    ;(err as Error & { code?: string }).code = code
    handlers.onError?.(err)
    return
  }

  const reader = res.body.getReader()
  const decoder = new TextDecoder()
  const feed = parseSSEStream(handlers.onEvent)
  try {
    for (;;) {
      const { done, value } = await reader.read()
      if (done) break
      feed(decoder.decode(value, { stream: true }))
    }
    feed(decoder.decode()) // flush
  } catch (err) {
    if ((err as Error).name !== 'AbortError') handlers.onError?.(err as Error)
  }
}
