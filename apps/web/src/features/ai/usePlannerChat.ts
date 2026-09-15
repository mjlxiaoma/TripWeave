import { useCallback, useEffect, useReducer, useRef } from 'react'
import { aiApi } from './api'
import type { ChatMessage, PlannerEvent, SummaryChange, ToolProgress } from './types'

export interface ChatState {
  messages: ChatMessage[]
  summary: SummaryChange[] | null
  summaryLines: string[]
  streaming: boolean
  error: string | null
  loaded: boolean
}

type Action =
  | { type: 'history'; messages: ChatMessage[] }
  | { type: 'send'; message: ChatMessage }
  | { type: 'token'; text: string }
  | { type: 'tool_start'; tool: ToolProgress }
  | { type: 'tool_result'; id: string; status: 'success' | 'error'; label: string }
  | { type: 'summary'; lines: string[]; changes: SummaryChange[] }
  | { type: 'finish' }
  | { type: 'error'; message: string }

const initialState: ChatState = {
  messages: [],
  summary: null,
  summaryLines: [],
  streaming: false,
  error: null,
  loaded: false,
}

function appendToLastAssistant(messages: ChatMessage[], text: string): ChatMessage[] {
  const last = messages[messages.length - 1]
  if (last && last.role === 'assistant') {
    const updated = { ...last, text: last.text + text, pending: true }
    return [...messages.slice(0, -1), updated]
  }
  return [...messages, { id: 'streaming', role: 'assistant', text, pending: true, tools: [] }]
}

function reducer(state: ChatState, action: Action): ChatState {
  switch (action.type) {
    case 'history':
      return { ...state, messages: action.messages, loaded: true }
    case 'send':
      return { ...state, messages: [...state.messages, action.message], streaming: true, error: null }
    case 'token':
      return { ...state, messages: appendToLastAssistant(state.messages, action.text) }
    case 'tool_start': {
      const messages = appendToLastAssistant(state.messages, '')
      const last = messages[messages.length - 1]
      const tools = [...(last.tools ?? []), action.tool]
      return { ...state, messages: [...messages.slice(0, -1), { ...last, tools }] }
    }
    case 'tool_result': {
      const messages = [...state.messages]
      const last = messages[messages.length - 1]
      if (last?.role === 'assistant') {
        const tools = (last.tools ?? []).map((t) =>
          t.id === action.id ? { ...t, status: action.status, label: action.label || t.label } : t,
        )
        messages[messages.length - 1] = { ...last, tools }
      }
      return { ...state, messages }
    }
    case 'summary':
      return { ...state, summary: action.changes, summaryLines: action.lines }
    case 'finish': {
      const messages = [...state.messages]
      const last = messages[messages.length - 1]
      if (last?.role === 'assistant') {
        messages[messages.length - 1] = { ...last, pending: false }
      }
      return { ...state, messages, streaming: false }
    }
    case 'error':
      return { ...state, streaming: false, error: action.message }
    default:
      return state
  }
}

export function usePlannerChat(
  tripId: string,
  onTripUpdated: () => void,
) {
  const [state, dispatch] = useReducer(reducer, initialState)
  const abortRef = useRef<AbortController | null>(null)
  const tripUpdatedRef = useRef(onTripUpdated)
  tripUpdatedRef.current = onTripUpdated
  const refreshTimer = useRef<ReturnType<typeof setTimeout> | null>(null)

  // 加载历史对话。
  useEffect(() => {
    let cancelled = false
    aiApi
      .listMessages(tripId)
      .then((res) => {
        if (cancelled) return
        const messages: ChatMessage[] = res.messages.map((m, i) => ({
          id: `h${i}`,
          role: m.role === 'assistant' ? 'assistant' : 'user',
          text: m.content,
        }))
        dispatch({ type: 'history', messages })
      })
      .catch(() => dispatch({ type: 'history', messages: [] }))
    return () => {
      cancelled = true
    }
  }, [tripId])

  const handleEvent = useCallback((e: PlannerEvent) => {
    switch (e.type) {
      case 'token':
        dispatch({ type: 'token', text: e.text })
        break
      case 'tool_start':
        dispatch({ type: 'tool_start', tool: { id: e.id, name: e.name, label: e.label, status: 'running' } })
        break
      case 'tool_result':
        dispatch({ type: 'tool_result', id: e.id, status: e.status, label: e.label })
        break
      case 'trip_updated':
        // debounce 合并一轮对话中的多次行程变更。
        if (refreshTimer.current) clearTimeout(refreshTimer.current)
        refreshTimer.current = setTimeout(() => tripUpdatedRef.current(), 200)
        break
      case 'summary':
        dispatch({ type: 'summary', lines: e.lines, changes: e.changes })
        break
      case 'done':
        dispatch({ type: 'finish' })
        tripUpdatedRef.current()
        break
      case 'error':
        dispatch({ type: 'error', message: e.message })
        break
      default:
        break
    }
  }, [])

  const send = useCallback(
    (text: string) => {
      const trimmed = text.trim()
      if (!trimmed || state.streaming) return
      abortRef.current?.abort()
      const controller = new AbortController()
      abortRef.current = controller
      dispatch({
        type: 'send',
        message: { id: `u${Date.now()}`, role: 'user', text: trimmed },
      })
      void aiApi.chatStream(tripId, trimmed, handleEvent, controller.signal)
    },
    [tripId, state.streaming, handleEvent],
  )

  const stop = useCallback(() => {
    abortRef.current?.abort()
    dispatch({ type: 'finish' })
  }, [])

  return { state, send, stop }
}
