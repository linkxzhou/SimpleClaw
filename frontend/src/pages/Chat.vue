<template>
  <div class="chat-page">
    <!-- Gateway not running warning -->
    <a-result
      v-if="gatewayStore.status.state !== 'running'"
      status="warning"
      :title="t('chat.gatewayNotRunning')"
      :sub-title="t('chat.gatewayRequired')"
    >
      <template #extra>
        <a-button type="primary" @click="$router.push('/settings')">
          {{ t('sidebar.settings') }}
        </a-button>
      </template>
    </a-result>

    <template v-else>
      <!-- Toolbar -->
      <div class="chat-toolbar">
        <a-select
          v-model:value="chatStore.currentSessionKey"
          style="width: 260px"
          @change="(val: string) => chatStore.switchSession(val)"
        >
          <a-select-option
            v-for="session in chatStore.sessions"
            :key="session.key"
            :value="session.key"
          >
            {{ session.displayName || session.label || session.key }}
          </a-select-option>
        </a-select>
        <a-button @click="chatStore.newSession">
          <template #icon><PlusOutlined /></template>
          {{ t('chat.toolbar.newSession') }}
        </a-button>
        <a-button @click="chatStore.refresh">
          <template #icon><ReloadOutlined /></template>
        </a-button>
        <a-button
          :type="chatStore.showThinking ? 'primary' : 'default'"
          @click="chatStore.toggleThinking"
        >
          <template #icon><BulbOutlined /></template>
          {{ chatStore.showThinking ? t('chat.toolbar.hideThinking') : t('chat.toolbar.showThinking') }}
        </a-button>
      </div>

      <!-- Messages -->
      <div class="chat-messages" ref="messagesRef">
        <!-- Welcome screen -->
        <div v-if="chatStore.messages.length === 0 && !chatStore.loading" class="welcome-screen">
          <div class="welcome-content">
            <h2>{{ t('chat.welcome.title') }}</h2>
            <p>{{ t('chat.welcome.subtitle') }}</p>
            <a-row :gutter="16" style="margin-top: 24px">
              <a-col :xs="24" :sm="12">
                <a-card hoverable size="small" style="margin-bottom: 12px">
                  <template #title>💡 {{ t('chat.welcome.askQuestions') }}</template>
                  <p>{{ t('chat.welcome.askQuestionsDesc') }}</p>
                </a-card>
              </a-col>
              <a-col :xs="24" :sm="12">
                <a-card hoverable size="small">
                  <template #title>✨ {{ t('chat.welcome.creativeTasks') }}</template>
                  <p>{{ t('chat.welcome.creativeTasksDesc') }}</p>
                </a-card>
              </a-col>
            </a-row>
          </div>
        </div>

        <!-- Loading -->
        <div v-if="chatStore.loading && chatStore.messages.length === 0" class="loading-center">
          <a-spin size="large" />
        </div>

        <!-- Message list -->
        <template v-for="(msg, idx) in chatStore.messages" :key="msg.id || idx">
          <ChatMessage :message="msg" :show-thinking="chatStore.showThinking" />
        </template>

        <!-- Streaming message -->
        <ChatMessage
          v-if="chatStore.streamingMessage"
          :message="chatStore.streamingMessage as any"
          :show-thinking="chatStore.showThinking"
          :is-streaming="true"
        />

        <!-- Tool status bar -->
        <div v-if="chatStore.streamingTools.length > 0" class="tool-status-bar">
          <a-space direction="vertical" :size="4" style="width: 100%">
            <div v-for="tool in chatStore.streamingTools" :key="tool.id || tool.name" class="tool-item">
              <a-badge
                :status="tool.status === 'running' ? 'processing' : tool.status === 'completed' ? 'success' : 'error'"
              />
              <span class="tool-name">{{ tool.name }}</span>
              <span v-if="tool.durationMs" class="tool-duration">{{ (tool.durationMs / 1000).toFixed(1) }}s</span>
              <span v-if="tool.summary" class="tool-summary">{{ tool.summary }}</span>
            </div>
          </a-space>
        </div>

        <!-- Typing indicator — AI-style loading bubble -->
        <div v-if="chatStore.sending && !chatStore.streamingMessage" class="typing-bubble">
          <div class="typing-avatar">🤖</div>
          <div class="typing-content">
            <span class="typing-dots">
              <span class="dot"></span>
              <span class="dot"></span>
              <span class="dot"></span>
            </span>
          </div>
        </div>
      </div>

      <!-- Error bar -->
      <a-alert
        v-if="chatStore.error"
        type="error"
        :message="chatStore.error"
        closable
        @close="chatStore.clearError"
        style="margin: 0 0 8px 0"
      />

      <!-- Input area -->
      <div class="chat-input-area">
        <div class="input-wrapper">
          <!-- Attachment previews -->
          <div v-if="attachedFiles.length > 0" class="attachment-preview-bar">
            <div v-for="(file, i) in attachedFiles" :key="i" class="attachment-preview-item">
              <img
                v-if="file.preview && file.mimeType.startsWith('image/')"
                :src="file.preview"
                class="attachment-thumb"
              />
              <PaperClipOutlined v-else class="attachment-file-icon" />
              <span class="attachment-name">{{ file.fileName }}</span>
              <CloseOutlined class="attachment-remove" @click="removeAttachment(i)" />
            </div>
          </div>

          <!-- Textarea -->
          <a-textarea
            v-model:value="inputText"
            :placeholder="t('chat.welcome.askQuestionsDesc')"
            :auto-size="{ minRows: 4, maxRows: 10 }"
            @keydown="handleKeydown"
            :disabled="chatStore.sending"
            class="input-textarea"
          />

          <!-- Bottom toolbar inside input box -->
          <div class="input-bottom-bar">
            <div class="input-actions-left">
              <a-tooltip :title="t('chat.input.attachImage')">
                <label class="attach-btn">
                  <PictureOutlined />
                  <input
                    type="file"
                    accept="image/*"
                    multiple
                    class="hidden-input"
                    @change="handleImageSelect"
                  />
                </label>
              </a-tooltip>
              <a-tooltip :title="t('chat.input.attachFile')">
                <label class="attach-btn">
                  <PaperClipOutlined />
                  <input
                    type="file"
                    multiple
                    class="hidden-input"
                    @change="handleFileSelect"
                  />
                </label>
              </a-tooltip>
            </div>
            <div class="input-actions-right">
              <a-button
                v-if="chatStore.sending"
                type="primary"
                danger
                size="small"
                @click="chatStore.abortRun"
                class="send-btn stop-btn-pulse"
                :loading="!chatStore.streamingMessage"
              >
                <template #icon><StopOutlined /></template>
                Stop
              </a-button>
              <a-button
                v-else
                type="primary"
                size="small"
                @click="handleSend"
                :disabled="!inputText.trim() && attachedFiles.length === 0"
                class="send-btn"
              >
                {{ t('chat.input.send') }}
              </a-button>
            </div>
          </div>
        </div>
      </div>
    </template>
  </div>
</template>

<script setup lang="ts">
import { ref, nextTick, watch, onMounted, onUnmounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { useChatStore } from '@/stores/chat'
import { useGatewayStore } from '@/stores/gateway'
import ChatMessage from '@/components/ChatMessage.vue'
import type { AttachedFileMeta } from '@/types/chat'
import {
  PlusOutlined, ReloadOutlined, BulbOutlined,
  StopOutlined, PictureOutlined,
  PaperClipOutlined, CloseOutlined,
} from '@ant-design/icons-vue'

const { t } = useI18n()
const chatStore = useChatStore()
const gatewayStore = useGatewayStore()
const inputText = ref('')
const messagesRef = ref<HTMLElement>()
const attachedFiles = ref<(AttachedFileMeta & { file?: File })[]>([])

function handleSend() {
  const text = inputText.value.trim()
  if (!text && attachedFiles.value.length === 0) return
  const files = attachedFiles.value.length > 0
    ? attachedFiles.value.map(f => ({
        fileName: f.fileName,
        mimeType: f.mimeType,
        fileSize: f.fileSize,
        stagedPath: f.filePath || '',
        preview: f.preview,
      }))
    : undefined
  inputText.value = ''
  attachedFiles.value = []
  chatStore.sendMessage(text || '(file attached)', files)
}

function handleKeydown(e: KeyboardEvent) {
  if (e.key === 'Enter' && !e.shiftKey && !e.isComposing) {
    e.preventDefault()
    handleSend()
  }
}

function handleImageSelect(e: Event) {
  const input = e.target as HTMLInputElement
  if (!input.files) return
  addFiles(Array.from(input.files))
  input.value = ''
}

function handleFileSelect(e: Event) {
  const input = e.target as HTMLInputElement
  if (!input.files) return
  addFiles(Array.from(input.files))
  input.value = ''
}

function addFiles(files: File[]) {
  for (const file of files) {
    const meta: AttachedFileMeta & { file?: File } = {
      fileName: file.name,
      mimeType: file.type || 'application/octet-stream',
      fileSize: file.size,
      preview: null,
      file,
    }
    if (file.type.startsWith('image/')) {
      const reader = new FileReader()
      reader.onload = () => {
        meta.preview = reader.result as string
        attachedFiles.value = [...attachedFiles.value]
      }
      reader.readAsDataURL(file)
    }
    attachedFiles.value.push(meta)
  }
}

function removeAttachment(index: number) {
  attachedFiles.value.splice(index, 1)
}

function scrollToBottom() {
  nextTick(() => {
    if (messagesRef.value) {
      messagesRef.value.scrollTop = messagesRef.value.scrollHeight
    }
  })
}

watch(() => chatStore.messages.length, scrollToBottom)
watch(() => chatStore.streamingMessage, scrollToBottom)

// 当 gateway 状态变为 running 时，自动加载会话和历史
watch(
  () => gatewayStore.status.state,
  async (newState) => {
    if (newState === 'running' && chatStore.messages.length === 0 && !chatStore.loading) {
      await chatStore.loadSessions()
      await chatStore.loadHistory()
      chatStore.connectEventSource()
    }
  }
)

onMounted(async () => {
  if (gatewayStore.status.state === 'running') {
    await chatStore.loadSessions()
    await chatStore.loadHistory()
    chatStore.connectEventSource()
  }
})

onUnmounted(() => {
  chatStore.disconnectEventSource()
})
</script>

<style scoped>
.chat-page { display: flex; flex-direction: column; height: 100%; max-width: 100%; }
.chat-toolbar { display: flex; gap: 8px; align-items: center; padding-bottom: 12px; flex-wrap: wrap; }
.chat-messages { flex: 1; overflow-y: auto; padding: 16px 0; -webkit-overflow-scrolling: touch; }
.welcome-screen { display: flex; align-items: center; justify-content: center; height: 100%; }
.welcome-content { text-align: center; max-width: 500px; }
.welcome-content h2 { color: var(--accent); }
.loading-center { display: flex; justify-content: center; padding: 40px; }

.tool-status-bar {
  padding: 8px 16px;
  background: var(--tool-status-bg);
  border-radius: var(--radius-md);
  margin: 8px 0;
  border: 1px solid var(--border-color);
}
.tool-item { display: flex; align-items: center; gap: 8px; font-size: 13px; }
.tool-name { font-weight: 500; }
.tool-duration { color: var(--text-tertiary); }
.tool-summary { color: var(--text-secondary); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; max-width: 300px; }

.typing-bubble { display: flex; align-items: flex-start; gap: 12px; padding: 12px 16px; }
.typing-avatar {
  width: 36px; height: 36px; border-radius: 50%;
  background: var(--bg-tertiary);
  border: 1px solid var(--glass-border);
  display: flex; align-items: center; justify-content: center;
  font-size: 20px; flex-shrink: 0;
}
.typing-content {
  background: var(--bg-tertiary);
  border-radius: 12px;
  padding: 14px 20px;
  display: flex;
  align-items: center;
  border: 1px solid var(--border-color);
}
.typing-dots { display: flex; gap: 5px; align-items: center; }
.typing-dots .dot {
  width: 8px; height: 8px; border-radius: 50%;
  background: var(--accent);
  animation: typingBounce 1.4s infinite ease-in-out both;
}
.typing-dots .dot:nth-child(1) { animation-delay: 0s; }
.typing-dots .dot:nth-child(2) { animation-delay: 0.2s; }
.typing-dots .dot:nth-child(3) { animation-delay: 0.4s; }
@keyframes typingBounce {
  0%, 80%, 100% { transform: scale(0.6); opacity: 0.4; }
  40% { transform: scale(1); opacity: 1; }
}

.stop-btn-pulse { animation: stopPulse 2s infinite ease-in-out; }
@keyframes stopPulse { 0%, 100% { opacity: 1; } 50% { opacity: 0.7; } }

.chat-input-area { padding: 12px 0 0; border-top: 1px solid var(--border-color); }
.input-wrapper {
  border: 1px solid var(--glass-border);
  border-radius: var(--radius-md);
  background: var(--input-bg);
  overflow: hidden;
  transition: border-color 0.2s;
}
.input-wrapper:focus-within {
  border-color: var(--accent);
  box-shadow: 0 0 0 2px var(--accent-dim);
}
.input-textarea :deep(.ant-input) {
  border: none !important;
  box-shadow: none !important;
  resize: none;
  background: transparent;
  padding: 12px 14px 4px;
}
.input-textarea :deep(.ant-input:focus) { box-shadow: none !important; }

.attachment-preview-bar { display: flex; flex-wrap: wrap; gap: 8px; padding: 10px 14px 0; }
.attachment-preview-item {
  display: flex; align-items: center; gap: 6px;
  background: var(--bg-tertiary);
  border-radius: var(--radius-sm);
  padding: 4px 8px;
  font-size: 12px;
  max-width: 180px;
}
.attachment-thumb { width: 28px; height: 28px; object-fit: cover; border-radius: 4px; }
.attachment-file-icon { font-size: 16px; color: var(--text-secondary); }
.attachment-name { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; flex: 1; min-width: 0; }
.attachment-remove { cursor: pointer; color: var(--text-tertiary); font-size: 10px; flex-shrink: 0; }
.attachment-remove:hover { color: #ff4d4f; }

.input-bottom-bar { display: flex; align-items: center; justify-content: space-between; padding: 4px 10px 8px; }
.input-actions-left { display: flex; gap: 4px; align-items: center; }
.input-actions-right { display: flex; align-items: center; }
.attach-btn {
  display: inline-flex; align-items: center; justify-content: center;
  width: 32px; height: 32px; border-radius: var(--radius-sm);
  cursor: pointer; color: var(--text-secondary);
  transition: background 0.2s, color 0.2s;
}
.attach-btn:hover { background: var(--bg-hover); color: var(--accent); }
.hidden-input { display: none; }
.send-btn { border-radius: 8px; height: 32px; padding: 0 16px; font-weight: 500; }

@media (max-width: 767px) {
  .chat-page { height: calc(100vh - 48px - 56px); max-width: 100%; }
  .chat-toolbar { gap: 6px; padding-bottom: 8px; }
  .chat-toolbar :deep(.ant-select) { width: 100% !important; min-width: 0; }
  .chat-toolbar :deep(.ant-btn) { padding: 0 8px; font-size: 12px; }
  .chat-toolbar :deep(.ant-btn span:not(.anticon)) { display: none; }
  .chat-messages { padding: 8px 0; }
  .welcome-content { padding: 0 16px; }
  .welcome-content h2 { font-size: 18px; }
  .tool-summary { max-width: 150px; }
  .chat-input-area { padding: 8px 0; padding-bottom: calc(8px + env(safe-area-inset-bottom, 0px)); }
  .send-btn { height: 30px; padding: 0 12px; }
  .typing-avatar { width: 28px; height: 28px; font-size: 16px; }
  .typing-content { padding: 10px 16px; }
  .typing-dots .dot { width: 6px; height: 6px; }
  .attachment-preview-item { max-width: 140px; }
}
</style>
