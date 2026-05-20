<script setup lang="ts">
import { ref, onMounted, onUnmounted } from 'vue'
import { api } from '@/composables/useApi'
import type { Agent } from '@/types/api'

const agents = ref<Agent[]>([])
const inputText = ref('')
const messages = ref<{ role: 'user' | 'assistant' | string; content: string }[]>([])
const thinking = ref(false)
const currentChatId = ref<string | null>(null)
const selectedAgent = ref('')
const textareaRef = ref<HTMLTextAreaElement | null>(null)
const messagesContainer = ref<HTMLDivElement | null>(null)

import { useStreamChat } from '@/composables/useStreamChat'
const { isStreaming, send: streamSend, abort: streamAbort } = useStreamChat()

onMounted(async () => {
  try {
    agents.value = await api.get<Agent[]>('/api/admin/agents')
    if (agents.value.length > 0) {
      selectedAgent.value = agents.value[0]!.name
    }
  } catch {
    // Admin API may require API key
  }
})

function adjustTextareaHeight() {
  const el = textareaRef.value
  if (!el) return
  el.style.height = 'auto'
  el.style.height = Math.min(el.scrollHeight, 150) + 'px'
}

function scrollToBottom() {
  const el = messagesContainer.value
  if (el) {
    el.scrollTop = el.scrollHeight
  }
}

async function sendMessage() {
  const text = inputText.value.trim()
  if (!text || isStreaming.value) return

  const chatId = currentChatId.value || crypto.randomUUID()
  currentChatId.value = chatId

  messages.value.push({ role: 'user', content: text })
  inputText.value = ''
  adjustTextareaHeight()

  const assistantIndex = messages.value.length
  messages.value.push({ role: 'assistant', content: '' })
  thinking.value = true
  scrollToBottom()

  streamSend(text, selectedAgent.value || '', chatId, {
    onEvent(type, data) {
      if (type === 'response' || type === 'error') {
        const content = data.content || data.error || ''
        const msg = messages.value[assistantIndex]
        if (msg) {
          msg.content = content
        }
        if (type === 'error') {
          messages.value[assistantIndex] = { role: 'error', content }
        }
      }
      if (type === 'done') {
        thinking.value = false
      }
    },
    onError(err) {
      const msg = messages.value[assistantIndex]
      if (msg) {
        msg.content = `Error: ${err}`
        msg.role = 'error'
      }
      thinking.value = false
    },
  })

  scrollToBottom()
}

function handleQuickAction(prompt: string) {
  inputText.value = prompt
  sendMessage()
}

function handleKeydown(e: KeyboardEvent) {
  if (e.key === 'Enter' && !e.shiftKey) {
    e.preventDefault()
    sendMessage()
  }
}

onUnmounted(() => {
  streamAbort()
})
</script>

<template>
  <div class="chat-container">
    <div ref="messagesContainer" class="chat-messages">
      <div v-if="messages.length === 0" class="chat-welcome">
        <div class="welcome-icon">⚡</div>
        <h1>neoclaw</h1>
        <p>Start a conversation with an AI agent</p>
        <div class="quick-actions">
          <div class="quick-action" @click="handleQuickAction('Explain how this system works')">
            How does this work?
          </div>
          <div class="quick-action" @click="handleQuickAction('Show me what agents are available')">
            Available agents
          </div>
        </div>
      </div>

      <div
        v-for="(msg, idx) in messages"
        :key="idx"
        class="message"
        :class="[msg.role, { error: msg.role === 'error' }]"
      >
        <div class="message-avatar">
          {{ msg.role === 'user' ? 'U' : 'AI' }}
        </div>
        <div class="message-bubble">{{ msg.content }}</div>
      </div>
    </div>

    <div v-show="thinking" class="thinking">
      <div class="thinking-dots"><span></span><span></span><span></span></div>
      <span>Thinking...</span>
    </div>

    <div class="chat-input-area">
      <div class="agent-selector-inline">
        <select id="agent-select" v-model="selectedAgent">
          <option v-if="agents.length === 0" value="">default</option>
          <option v-for="agent in agents" :key="agent.name" :value="agent.name">
            {{ agent.name }}
          </option>
        </select>
      </div>
      <textarea
        ref="textareaRef"
        v-model="inputText"
        placeholder="Type a message..."
        rows="1"
        @input="adjustTextareaHeight"
        @keydown="handleKeydown"
      ></textarea>
      <button
        class="send-btn"
        :disabled="isStreaming || !inputText.trim()"
        @click="sendMessage"
      >
        Send
      </button>
    </div>
  </div>
</template>
