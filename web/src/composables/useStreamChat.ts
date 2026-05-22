import { ref } from 'vue'
import { api } from './useApi'
import type { ChatEvent } from '@/types/api'

export function useStreamChat() {
  const isStreaming = ref(false)
  let abortController: AbortController | null = null

  function send(
    message: string,
    agentName: string,
    chatId: string,
    callbacks: {
      onEvent?: (type: string, data: ChatEvent) => void
      onError?: (err: string) => void
    },
  ) {
    if (isStreaming.value) return

    const controller = new AbortController()
    abortController = controller
    isStreaming.value = true

    const url = `${api['baseUrl']}/api/chat`

    fetch(url, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ message, agent_name: agentName, chat_id: chatId }),
      signal: controller.signal,
    })
      .then(async (res) => {
        if (!res.ok) {
          callbacks.onError?.(await res.text())
          return
        }

        const reader = res.body!.getReader()
        const decoder = new TextDecoder()
        let buffer = ''

        while (true) {
          const { done, value } = await reader.read()
          if (done) break

          buffer += decoder.decode(value, { stream: true })
          const lines = buffer.split('\n')
          buffer = lines.pop() || ''

          let eventType = ''
          let dataStr = ''

          for (const line of lines) {
            if (line.startsWith('event: ')) {
              eventType = line.slice(7)
            } else if (line.startsWith('data: ')) {
              dataStr = line.slice(6)
              if (eventType && dataStr) {
                try {
                  const data = JSON.parse(dataStr) as ChatEvent
                  callbacks.onEvent?.(eventType, data)
                } catch {
                  // skip malformed data
                }
                eventType = ''
                dataStr = ''
              }
            }
          }
        }
      })
      .catch((err) => {
        if ((err as Error).name !== 'AbortError') {
          callbacks.onError?.((err as Error).message)
        }
      })
      .finally(() => {
        isStreaming.value = false
        abortController = null
      })

    return controller
  }

  function abort() {
    if (abortController) {
      abortController.abort()
      abortController = null
    }
    isStreaming.value = false
  }

  function reset() {
    isStreaming.value = false
    abortController = null
  }

  return {
    isStreaming,
    send,
    abort,
    reset,
  }
}
