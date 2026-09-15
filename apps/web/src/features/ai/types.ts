// AI 对话相关的类型,与后端 SSE 事件协议对应(docs/API.md)。

export interface SummaryChange {
  day_number: number
  kind: 'created' | 'updated' | 'deleted' | 'trip_info'
  op?: string
  title?: string
  field?: string
  from?: string
  to?: string
}

export type PlannerEvent =
  | { type: 'meta'; conversation_id: string; message_id: string; task_id: string }
  | { type: 'token'; text: string }
  | { type: 'tool_start'; id: string; name: string; label: string }
  | { type: 'tool_result'; id: string; name: string; status: 'success' | 'error'; label: string }
  | { type: 'trip_updated'; revision: number; changed_days: string[] }
  | { type: 'summary'; lines: string[]; changes: SummaryChange[] }
  | {
      type: 'done'
      message_id: string
      usage?: { prompt_tokens: number; completion_tokens: number }
      finish_reason: string
    }
  | { type: 'error'; code: string; message: string }

export interface ToolProgress {
  id: string
  name: string
  label: string
  status: 'running' | 'success' | 'error'
}

export interface ChatMessage {
  id: string
  role: 'user' | 'assistant'
  text: string
  pending?: boolean
  tools?: ToolProgress[]
}

export interface HistoryMessage {
  role: string
  content: string
}
