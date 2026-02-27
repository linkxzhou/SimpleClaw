<template>
  <div :class="['chat-message', `chat-message--${role}`, { streaming: isStreaming }]">
    <!-- Avatar + bubble row -->
    <div class="message-row">
      <!-- Avatar -->
      <div v-if="role !== 'system'" class="avatar" :class="`avatar--${role}`">
        <span v-if="role === 'user'">U</span>
        <span v-else class="avatar-emoji">🤖</span>
      </div>

      <div class="message-body">
        <div class="message-content">
          <!-- User message -->
          <template v-if="role === 'user'">
            <div class="user-bubble">
              <div class="message-text" v-html="renderText(textContent)" />
              <!-- Attached files -->
              <div v-if="attachedFiles.length > 0" class="attachments">
                <div v-for="(file, i) in attachedFiles" :key="i" class="attachment-item">
                  <img
                    v-if="file.preview && file.mimeType?.startsWith('image/')"
                    :src="file.preview"
                    :alt="file.fileName"
                    class="attachment-image"
                    @click="previewImage(file.preview!)"
                  />
                  <a-tag v-else>
                    <PaperClipOutlined /> {{ file.fileName }}
                  </a-tag>
                </div>
              </div>
            </div>
          </template>

          <!-- Assistant message -->
          <template v-else-if="role === 'assistant'">
            <div class="assistant-bubble">
              <!-- Thinking block -->
              <a-collapse
                v-if="showThinking && thinkingContent"
                :bordered="false"
                class="thinking-block"
              >
                <a-collapse-panel key="thinking" header="💭 Thinking...">
                  <pre class="thinking-text">{{ thinkingContent }}</pre>
                </a-collapse-panel>
              </a-collapse>

              <!-- Main text -->
              <div v-if="textContent" class="message-text markdown-body" v-html="renderMarkdown(textContent)" />

              <!-- Tool calls -->
              <div v-if="toolCalls.length > 0" class="tool-calls">
                <a-collapse :bordered="false" size="small">
                  <a-collapse-panel
                    v-for="tool in toolCalls"
                    :key="tool.id || tool.name"
                    :header="`🔧 ${tool.name || 'Tool Call'}`"
                  >
                    <pre class="tool-input">{{ formatToolInput(tool.input || tool.arguments) }}</pre>
                  </a-collapse-panel>
                </a-collapse>
              </div>

              <!-- Images from tool results -->
              <div v-if="attachedFiles.length > 0" class="attachments">
                <div v-for="(file, i) in attachedFiles" :key="i" class="attachment-item">
                  <img
                    v-if="file.preview && file.mimeType?.startsWith('image/')"
                    :src="file.preview"
                    :alt="file.fileName"
                    class="attachment-image"
                    @click="previewImage(file.preview!)"
                  />
                  <a-tag v-else-if="file.fileName !== 'image'">
                    <PaperClipOutlined /> {{ file.fileName }}
                  </a-tag>
                </div>
              </div>

              <!-- Streaming cursor -->
              <span v-if="isStreaming" class="streaming-cursor">▊</span>
            </div>
          </template>

          <!-- System message -->
          <template v-else>
            <div class="system-bubble">
              <div class="message-text" v-html="renderText(textContent)" />
            </div>
          </template>
        </div>

        <!-- Timestamp -->
        <div v-if="timestamp" class="message-time">
          {{ formatTime(timestamp) }}
        </div>
      </div>
    </div>
  </div>

  <!-- Image preview modal -->
  <a-modal
    v-model:open="imagePreviewVisible"
    :footer="null"
    :width="isMobile ? '95vw' : 800"
    centered
  >
    <img :src="imagePreviewUrl" style="width: 100%" />
  </a-modal>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { PaperClipOutlined } from '@ant-design/icons-vue'
import { useSettingsStore } from '@/stores/settings'
import type { RawMessage, ContentBlock, AttachedFileMeta } from '@/types/chat'
import MarkdownIt from 'markdown-it'

const props = defineProps<{
  message: RawMessage
  showThinking?: boolean
  isStreaming?: boolean
}>()

const settingsStore = useSettingsStore()
const isMobile = computed(() => settingsStore.isMobile)

const md = new MarkdownIt({
  html: false,
  linkify: true,
  typographer: true,
})

const imagePreviewVisible = ref(false)
const imagePreviewUrl = ref('')

const role = computed(() => props.message.role || 'assistant')
const timestamp = computed(() => props.message.timestamp)

const textContent = computed(() => {
  const content = props.message.content
  if (typeof content === 'string') return content
  if (Array.isArray(content)) {
    return (content as ContentBlock[])
      .filter(b => b.type === 'text' && b.text)
      .map(b => b.text!)
      .join('\n')
  }
  return ''
})

const thinkingContent = computed(() => {
  const content = props.message.content
  if (!Array.isArray(content)) return ''
  return (content as ContentBlock[])
    .filter(b => b.type === 'thinking' && b.thinking)
    .map(b => b.thinking!)
    .join('\n')
})

const toolCalls = computed(() => {
  const content = props.message.content
  if (!Array.isArray(content)) return []
  return (content as ContentBlock[]).filter(b => b.type === 'tool_use' || b.type === 'toolCall')
})

const attachedFiles = computed<AttachedFileMeta[]>(() => {
  return props.message._attachedFiles || []
})

function renderMarkdown(text: string): string {
  return md.render(text)
}

function renderText(text: string): string {
  return text.replace(/\n/g, '<br>')
}

function formatToolInput(input: unknown): string {
  if (typeof input === 'string') {
    try { return JSON.stringify(JSON.parse(input), null, 2) } catch { return input }
  }
  if (input && typeof input === 'object') return JSON.stringify(input, null, 2)
  return String(input || '')
}

function formatTime(ts: number): string {
  const date = new Date(ts > 1e12 ? ts : ts * 1000)
  const now = new Date()
  const isToday = date.toDateString() === now.toDateString()
  if (isToday) {
    return date.toLocaleTimeString(undefined, { hour: '2-digit', minute: '2-digit' })
  }
  return date.toLocaleDateString(undefined, { month: 'short', day: 'numeric' }) + ' ' +
    date.toLocaleTimeString(undefined, { hour: '2-digit', minute: '2-digit' })
}

function previewImage(url: string) {
  imagePreviewUrl.value = url
  imagePreviewVisible.value = true
}
</script>

<style scoped>
.chat-message { margin-bottom: 20px; display: flex; flex-direction: column; }
.chat-message--user { align-items: flex-end; }
.chat-message--assistant, .chat-message--system { align-items: flex-start; }

.message-row { display: flex; gap: 10px; align-items: flex-start; max-width: 85%; }
.chat-message--user .message-row { flex-direction: row-reverse; }
.chat-message--system .message-row { max-width: 90%; }

.avatar {
  width: 36px; height: 36px; border-radius: 50%;
  display: flex; align-items: center; justify-content: center;
  font-size: 13px; font-weight: 600; flex-shrink: 0; margin-top: 2px;
}
.avatar--user {
  background: var(--user-bubble-bg);
  color: #fff;
  box-shadow: 0 0 10px rgba(0, 184, 212, 0.3);
}
.avatar--assistant {
  background: var(--bg-tertiary);
  border: 1px solid var(--glass-border);
  color: var(--accent);
}
.avatar-emoji { font-size: 20px; line-height: 1; }

.message-body { display: flex; flex-direction: column; min-width: 0; }
.chat-message--user .message-body { align-items: flex-end; }
.chat-message--assistant .message-body { align-items: flex-start; }

.user-bubble {
  background: var(--user-bubble-bg);
  color: white;
  padding: 10px 16px;
  border-radius: 16px 16px 4px 16px;
  max-width: 100%;
  word-break: break-word;
  box-shadow: 0 2px 12px rgba(0, 145, 234, 0.2);
}
.assistant-bubble {
  background: var(--assistant-bubble-bg);
  color: var(--assistant-bubble-text);
  padding: 10px 16px;
  border-radius: 16px 16px 16px 4px;
  max-width: 100%;
  word-break: break-word;
  border: 1px solid var(--border-color);
}
.system-bubble {
  background: var(--system-bubble-bg);
  padding: 8px 12px;
  border-radius: 8px;
  font-size: 13px;
  color: var(--system-bubble-text);
  max-width: 100%;
  border: 1px solid var(--border-color);
}
.message-text { line-height: 1.6; }
.message-text :deep(pre) {
  background: #0d1117;
  color: #c9d1d9;
  padding: 12px;
  border-radius: var(--radius-sm);
  overflow-x: auto;
  font-size: 13px;
  -webkit-overflow-scrolling: touch;
  border: 1px solid rgba(0, 229, 255, 0.1);
}
.message-text :deep(code) {
  background: var(--code-bg);
  padding: 2px 4px;
  border-radius: 3px;
  font-size: 0.9em;
}
.message-text :deep(pre code) { background: transparent; padding: 0; }
.message-text :deep(img) { max-width: 100%; height: auto; }
.message-text :deep(table) { display: block; overflow-x: auto; -webkit-overflow-scrolling: touch; }

.message-time { font-size: 11px; color: var(--text-tertiary); margin-top: 4px; padding: 0 4px; }
.thinking-block { margin-bottom: 8px; }
.thinking-text { font-size: 13px; color: var(--text-secondary); white-space: pre-wrap; word-break: break-word; }
.tool-calls { margin-top: 8px; }
.tool-input { font-size: 12px; white-space: pre-wrap; word-break: break-all; max-height: 200px; overflow-y: auto; }

.attachments { display: flex; flex-wrap: wrap; gap: 8px; margin-top: 8px; }
.attachment-image { max-width: 200px; max-height: 200px; border-radius: 8px; cursor: pointer; object-fit: cover; }

.streaming-cursor { animation: blink 0.8s infinite; color: var(--accent); }
@keyframes blink { 0%, 50% { opacity: 1; } 51%, 100% { opacity: 0; } }

@media (max-width: 767px) {
  .message-row { gap: 8px; max-width: 92%; }
  .avatar { width: 30px; height: 30px; font-size: 11px; }
  .avatar .avatar-emoji { font-size: 16px; }
  .user-bubble, .assistant-bubble { padding: 8px 12px; }
  .system-bubble { max-width: 95%; }
  .message-text { font-size: 14px; line-height: 1.5; }
  .message-text :deep(pre) { font-size: 12px; padding: 8px; max-width: calc(100vw - 80px); }
  .attachment-image { max-width: 150px; max-height: 150px; }
  .tool-input { max-height: 150px; font-size: 11px; }
}
</style>
