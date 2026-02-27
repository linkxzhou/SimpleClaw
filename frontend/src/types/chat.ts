export interface AttachedFileMeta {
  fileName: string
  mimeType: string
  fileSize: number
  preview: string | null
  filePath?: string
}

export interface RawMessage {
  role: 'user' | 'assistant' | 'system' | 'toolresult'
  content: unknown
  timestamp?: number
  id?: string
  toolCallId?: string
  toolName?: string
  details?: unknown
  isError?: boolean
  _attachedFiles?: AttachedFileMeta[]
}

export interface ContentBlock {
  type: 'text' | 'image' | 'thinking' | 'tool_use' | 'tool_result' | 'toolCall' | 'toolResult'
  text?: string
  thinking?: string
  source?: { type: string; media_type?: string; data?: string; url?: string }
  data?: string
  mimeType?: string
  id?: string
  name?: string
  input?: unknown
  arguments?: unknown
  content?: unknown
}

export interface ChatSession {
  key: string
  label?: string
  displayName?: string
  thinkingLevel?: string
  model?: string
}

export interface ToolStatus {
  id?: string
  toolCallId?: string
  name: string
  status: 'running' | 'completed' | 'error'
  durationMs?: number
  summary?: string
  updatedAt: number
}
