import { defineStore } from 'pinia'
import { ref } from 'vue'
import type { RawMessage, ChatSession, ToolStatus, AttachedFileMeta, ContentBlock } from '@/types/chat'
import { sendChatMessage, getChatHistory, abortChat, listSessions } from '@/api/gateway'

const DEFAULT_CANONICAL_PREFIX = 'agent:main'
const DEFAULT_SESSION_KEY = `${DEFAULT_CANONICAL_PREFIX}:main`

function getMessageText(content: unknown): string {
  if (typeof content === 'string') return content
  if (Array.isArray(content)) {
    return (content as Array<{ type?: string; text?: string }>)
      .filter(b => b.type === 'text' && b.text)
      .map(b => b.text!)
      .join('\n')
  }
  return ''
}

function isToolResultRole(role: unknown): boolean {
  if (!role) return false
  const normalized = String(role).toLowerCase()
  return normalized === 'toolresult' || normalized === 'tool_result'
}

function isToolOnlyMessage(message: RawMessage | undefined): boolean {
  if (!message) return false
  if (isToolResultRole(message.role)) return true
  const content = message.content
  if (!Array.isArray(content)) return false
  let hasTool = false
  let hasText = false
  for (const block of content as ContentBlock[]) {
    if (block.type === 'tool_use' || block.type === 'tool_result' || block.type === 'toolCall' || block.type === 'toolResult') {
      hasTool = true
      continue
    }
    if (block.type === 'text' && block.text && block.text.trim()) {
      hasText = true
    }
  }
  return hasTool && !hasText
}

function hasNonToolAssistantContent(message: RawMessage | undefined): boolean {
  if (!message) return false
  if (typeof message.content === 'string' && message.content.trim()) return true
  const content = message.content
  if (Array.isArray(content)) {
    for (const block of content as ContentBlock[]) {
      if (block.type === 'text' && block.text && block.text.trim()) return true
      if (block.type === 'thinking' && block.thinking && block.thinking.trim()) return true
      if (block.type === 'image') return true
    }
  }
  return false
}

function extractImagesAsAttachedFiles(content: unknown): AttachedFileMeta[] {
  if (!Array.isArray(content)) return []
  const files: AttachedFileMeta[] = []
  for (const block of content as ContentBlock[]) {
    if (block.type === 'image') {
      if (block.source) {
        const src = block.source
        const mimeType = src.media_type || 'image/jpeg'
        if (src.type === 'base64' && src.data) {
          files.push({ fileName: 'image', mimeType, fileSize: 0, preview: `data:${mimeType};base64,${src.data}` })
        } else if (src.type === 'url' && src.url) {
          files.push({ fileName: 'image', mimeType, fileSize: 0, preview: src.url })
        }
      } else if (block.data) {
        const mimeType = block.mimeType || 'image/jpeg'
        files.push({ fileName: 'image', mimeType, fileSize: 0, preview: `data:${mimeType};base64,${block.data}` })
      }
    }
    if ((block.type === 'tool_result' || block.type === 'toolResult') && block.content) {
      files.push(...extractImagesAsAttachedFiles(block.content))
    }
  }
  return files
}

function collectToolUpdates(message: unknown, eventState: string): ToolStatus[] {
  if (!message || typeof message !== 'object') return []
  const msg = message as Record<string, unknown>
  const updates: ToolStatus[] = []
  const content = msg.content
  if (Array.isArray(content)) {
    for (const block of content as ContentBlock[]) {
      if ((block.type === 'tool_use' || block.type === 'toolCall') && block.name) {
        updates.push({
          id: block.id || block.name,
          toolCallId: block.id,
          name: block.name,
          status: 'running',
          updatedAt: Date.now(),
        })
      }
      if ((block.type === 'tool_result' || block.type === 'toolResult') && block.name) {
        updates.push({
          id: block.id || block.name,
          toolCallId: block.id,
          name: block.name || 'tool',
          status: eventState === 'delta' ? 'running' : 'completed',
          updatedAt: Date.now(),
        })
      }
    }
  }
  // Check for tool_result role message
  const role = typeof msg.role === 'string' ? msg.role.toLowerCase() : ''
  if (isToolResultRole(role)) {
    const toolName = typeof msg.toolName === 'string' ? msg.toolName : (typeof msg.name === 'string' ? msg.name : 'tool')
    const toolCallId = typeof msg.toolCallId === 'string' ? msg.toolCallId : undefined
    updates.push({
      id: toolCallId || toolName,
      toolCallId,
      name: toolName,
      status: eventState === 'delta' ? 'running' : 'completed',
      updatedAt: Date.now(),
    })
  }
  return updates
}

function upsertToolStatuses(current: ToolStatus[], updates: ToolStatus[]): ToolStatus[] {
  if (updates.length === 0) return current
  const next = [...current]
  for (const update of updates) {
    const key = update.toolCallId || update.id || update.name
    if (!key) continue
    const index = next.findIndex((tool) => (tool.toolCallId || tool.id || tool.name) === key)
    if (index === -1) {
      next.push(update)
    } else {
      const existing = next[index]
      next[index] = {
        ...existing,
        ...update,
        name: update.name || existing.name,
        durationMs: update.durationMs ?? existing.durationMs,
        summary: update.summary ?? existing.summary,
      }
    }
  }
  return next
}

export const useChatStore = defineStore('chat', () => {
  const messages = ref<RawMessage[]>([])
  const loading = ref(false)
  const error = ref<string | null>(null)
  const sending = ref(false)
  const activeRunId = ref<string | null>(null)
  const streamingText = ref('')
  const streamingMessage = ref<unknown | null>(null)
  const streamingTools = ref<ToolStatus[]>([])
  const pendingFinal = ref(false)
  const lastUserMessageAt = ref<number | null>(null)
  const pendingToolImages = ref<AttachedFileMeta[]>([])
  const sessions = ref<ChatSession[]>([])
  const currentSessionKey = ref(DEFAULT_SESSION_KEY)
  const showThinking = ref(true)
  const thinkingLevel = ref<string | null>(null)

  let eventSource: EventSource | null = null

  async function loadSessions() {
    try {
      const data = await listSessions()
      const rawSessions = Array.isArray(data.sessions) ? data.sessions : []
      const parsed: ChatSession[] = (rawSessions as Record<string, unknown>[]).map((s) => ({
        key: String(s.key || ''),
        label: s.label ? String(s.label) : undefined,
        displayName: s.displayName ? String(s.displayName) : undefined,
        thinkingLevel: s.thinkingLevel ? String(s.thinkingLevel) : undefined,
        model: s.model ? String(s.model) : undefined,
      })).filter((s) => s.key)

      // Deduplicate
      const seen = new Set<string>()
      const deduped = parsed.filter((s) => {
        if (seen.has(s.key)) return false
        seen.add(s.key)
        return true
      })

      let nextKey = currentSessionKey.value || DEFAULT_SESSION_KEY
      if (!deduped.find((s) => s.key === nextKey) && deduped.length > 0) {
        nextKey = deduped[0].key
      }

      const sessionsWithCurrent = !deduped.find((s) => s.key === nextKey) && nextKey
        ? [...deduped, { key: nextKey, displayName: nextKey }]
        : deduped

      sessions.value = sessionsWithCurrent
      if (currentSessionKey.value !== nextKey) {
        currentSessionKey.value = nextKey
        await loadHistory()
      }
    } catch (err) {
      console.warn('Failed to load sessions:', err)
    }
  }

  function switchSession(key: string) {
    currentSessionKey.value = key
    messages.value = []
    streamingText.value = ''
    streamingMessage.value = null
    streamingTools.value = []
    activeRunId.value = null
    error.value = null
    pendingFinal.value = false
    pendingToolImages.value = []
    loadHistory()
    connectEventSource()
  }

  function newSession() {
    const prefix = sessions.value.find(s => s.key.startsWith('agent:'))?.key.split(':').slice(0, 2).join(':') ?? DEFAULT_CANONICAL_PREFIX
    const newKey = `${prefix}:session-${Date.now()}`
    sessions.value = [...sessions.value, { key: newKey, displayName: newKey }]
    switchSession(newKey)
  }

  async function loadHistory(quiet = false) {
    if (!quiet) {
      loading.value = true
      error.value = null
    }
    try {
      const data = await getChatHistory(currentSessionKey.value)
      const rawMessages = Array.isArray(data.messages) ? data.messages as RawMessage[] : []
      // Enrich: attach images from tool results to following assistant messages
      const enriched = enrichWithToolResultFiles(rawMessages)
      const filtered = enriched.filter((msg) => !isToolResultRole(msg.role))
      // 确保每条消息都有时间戳
      const now = Date.now() / 1000
      for (let i = 0; i < filtered.length; i++) {
        if (!filtered[i].timestamp) {
          filtered[i] = { ...filtered[i], timestamp: now }
        }
      }
      messages.value = filtered
      thinkingLevel.value = data.thinkingLevel ?? null
    } catch (err) {
      console.warn('Failed to load chat history:', err)
      if (!quiet) messages.value = []
    } finally {
      loading.value = false
    }
  }

  function enrichWithToolResultFiles(msgs: RawMessage[]): RawMessage[] {
    const pending: AttachedFileMeta[] = []
    return msgs.map((msg) => {
      if (isToolResultRole(msg.role)) {
        pending.push(...extractImagesAsAttachedFiles(msg.content))
        return msg
      }
      if (msg.role === 'assistant' && pending.length > 0) {
        const toAttach = pending.splice(0)
        return { ...msg, _attachedFiles: [...(msg._attachedFiles || []), ...toAttach] }
      }
      return msg
    })
  }

  async function sendMessage(text: string, attachments?: Array<{ fileName: string; mimeType: string; fileSize: number; stagedPath: string; preview: string | null }>) {
    const trimmed = text.trim()
    if (!trimmed && (!attachments || attachments.length === 0)) return

    const userMsg: RawMessage = {
      role: 'user',
      content: trimmed || '(file attached)',
      timestamp: Date.now() / 1000,
      id: crypto.randomUUID(),
      _attachedFiles: attachments?.map(a => ({
        fileName: a.fileName,
        mimeType: a.mimeType,
        fileSize: a.fileSize,
        preview: a.preview,
        filePath: a.stagedPath,
      })),
    }
    messages.value = [...messages.value, userMsg]
    sending.value = true
    error.value = null
    streamingText.value = ''
    streamingMessage.value = null
    streamingTools.value = []
    pendingFinal.value = false
    lastUserMessageAt.value = userMsg.timestamp ?? null

    try {
      const media = attachments?.map(a => ({
        filePath: a.stagedPath,
        mimeType: a.mimeType,
        fileName: a.fileName,
      }))
      const result = await sendChatMessage(currentSessionKey.value, trimmed, media)
      if (result.runId) {
        activeRunId.value = result.runId
      }
      // Safety timeout
      const sentAt = Date.now()
      const check = () => {
        if (!sending.value) return
        if (streamingMessage.value || streamingText.value) return
        if (Date.now() - sentAt < 90_000) {
          setTimeout(check, 10_000)
          return
        }
        error.value = 'No response received from the model. Please check your provider settings.'
        sending.value = false
        activeRunId.value = null
      }
      setTimeout(check, 30_000)
    } catch (err) {
      error.value = String(err)
      sending.value = false
    }
  }

  async function abortRun() {
    sending.value = false
    streamingText.value = ''
    streamingMessage.value = null
    streamingTools.value = []
    pendingFinal.value = false
    pendingToolImages.value = []
    try {
      await abortChat(currentSessionKey.value)
    } catch (err) {
      error.value = String(err)
    }
  }

  function handleChatEvent(event: Record<string, unknown>) {
    const runId = String(event.runId || '')
    const eventState = String(event.state || '')

    if (activeRunId.value && runId && runId !== activeRunId.value) return

    let resolvedState = eventState
    if (!resolvedState && event.message && typeof event.message === 'object') {
      const msg = event.message as Record<string, unknown>
      const stopReason = msg.stopReason ?? msg.stop_reason
      resolvedState = stopReason ? 'final' : (msg.role || msg.content) ? 'delta' : ''
    }

    switch (resolvedState) {
      case 'delta': {
        const updates = collectToolUpdates(event.message, resolvedState)
        if (event.message && typeof event.message === 'object') {
          const msgRole = (event.message as RawMessage).role
          if (!isToolResultRole(msgRole)) {
            streamingMessage.value = event.message
          }
        }
        if (updates.length > 0) {
          streamingTools.value = upsertToolStatuses(streamingTools.value, updates)
        }
        break
      }
      case 'final': {
        const finalMsg = event.message as RawMessage | undefined
        if (finalMsg) {
          const updates = collectToolUpdates(finalMsg, resolvedState)
          if (isToolResultRole(finalMsg.role)) {
            const toolFiles = extractImagesAsAttachedFiles(finalMsg.content)
            streamingText.value = ''
            streamingMessage.value = null
            pendingFinal.value = true
            pendingToolImages.value = [...pendingToolImages.value, ...toolFiles]
            if (updates.length > 0) {
              streamingTools.value = upsertToolStatuses(streamingTools.value, updates)
            }
            break
          }
          const toolOnly = isToolOnlyMessage(finalMsg)
          const hasOutput = hasNonToolAssistantContent(finalMsg)
          const msgId = finalMsg.id || `run-${runId}-${Date.now()}`
          const pendingImgs = pendingToolImages.value
          const msgWithImages: RawMessage = pendingImgs.length > 0
            ? { ...finalMsg, role: (finalMsg.role || 'assistant') as RawMessage['role'], id: msgId, timestamp: finalMsg.timestamp || Date.now() / 1000, _attachedFiles: [...(finalMsg._attachedFiles || []), ...pendingImgs] }
            : { ...finalMsg, role: (finalMsg.role || 'assistant') as RawMessage['role'], id: msgId, timestamp: finalMsg.timestamp || Date.now() / 1000 }
          const alreadyExists = messages.value.some(m => m.id === msgId)
          if (!alreadyExists) {
            messages.value = [...messages.value, msgWithImages]
          }
          streamingText.value = ''
          streamingMessage.value = null
          pendingToolImages.value = []
          if (hasOutput && !toolOnly) {
            sending.value = false
            activeRunId.value = null
            pendingFinal.value = false
            streamingTools.value = []
          } else {
            pendingFinal.value = true
            if (updates.length > 0) {
              streamingTools.value = upsertToolStatuses(streamingTools.value, updates)
            }
          }
          if (hasOutput && !toolOnly) {
            loadHistory(true)
          }
        } else {
          streamingText.value = ''
          streamingMessage.value = null
          pendingFinal.value = true
          loadHistory()
        }
        break
      }
      case 'error': {
        error.value = String(event.errorMessage || 'An error occurred')
        sending.value = false
        activeRunId.value = null
        streamingText.value = ''
        streamingMessage.value = null
        streamingTools.value = []
        pendingFinal.value = false
        pendingToolImages.value = []
        break
      }
      case 'aborted': {
        sending.value = false
        activeRunId.value = null
        streamingText.value = ''
        streamingMessage.value = null
        streamingTools.value = []
        pendingFinal.value = false
        pendingToolImages.value = []
        break
      }
      default: {
        if (sending.value && event.message && typeof event.message === 'object') {
          const updates = collectToolUpdates(event.message, 'delta')
          streamingMessage.value = event.message
          if (updates.length > 0) {
            streamingTools.value = upsertToolStatuses(streamingTools.value, updates)
          }
        }
      }
    }
  }

  function connectEventSource() {
    if (eventSource) {
      eventSource.close()
      eventSource = null
    }
    try {
      const es = new EventSource(`/api/events?sessionKey=${encodeURIComponent(currentSessionKey.value)}`)
      es.onmessage = (e) => {
        try {
          const event = JSON.parse(e.data)
          handleChatEvent(event)
        } catch { /* ignore parse errors */ }
      }
      es.onerror = () => {
        // Will auto-reconnect
      }
      eventSource = es
    } catch { /* SSE not available, fall back to polling */ }
  }

  function disconnectEventSource() {
    if (eventSource) {
      eventSource.close()
      eventSource = null
    }
  }

  function toggleThinking() {
    showThinking.value = !showThinking.value
  }

  async function refresh() {
    await Promise.all([loadHistory(), loadSessions()])
  }

  function clearError() {
    error.value = null
  }

  return {
    messages, loading, error, sending, activeRunId,
    streamingText, streamingMessage, streamingTools,
    pendingFinal, lastUserMessageAt, pendingToolImages,
    sessions, currentSessionKey, showThinking, thinkingLevel,
    loadSessions, switchSession, newSession, loadHistory,
    sendMessage, abortRun, handleChatEvent,
    connectEventSource, disconnectEventSource,
    toggleThinking, refresh, clearError,
  }
})
