import { api } from '../../services/api'
import { streamSSE } from '../../services/sse'
import type { HistoryMessage, PlannerEvent } from './types'

interface MessagesResponse {
  conversation_id: string
  messages: HistoryMessage[]
}

export const aiApi = {
  // 流式对话:每个 SSE 事件回调给 onEvent。
  chatStream: (
    tripId: string,
    message: string,
    onEvent: (e: PlannerEvent) => void,
    signal?: AbortSignal,
  ): Promise<void> =>
    streamSSE(
      `/trips/${tripId}/ai/chat`,
      { message },
      {
        onEvent: (raw) => {
          try {
            onEvent({ type: raw.event, ...JSON.parse(raw.data) } as PlannerEvent)
          } catch {
            /* 忽略无法解析的事件 */
          }
        },
        onError: (err) => {
          onEvent({
            type: 'error',
            code: (err as Error & { code?: string }).code ?? 'NETWORK',
            message: err.message,
          })
        },
      },
      signal,
    ),

  listMessages: (tripId: string, limit = 50) =>
    api<MessagesResponse>(`/trips/${tripId}/ai/messages?limit=${limit}`),
}
