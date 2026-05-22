import type { App } from 'vue'
import { reportError } from '@/services/errorReporter'

// Global handler for window.onerror
function handleWindowError(
  msg: string | Event,
  url?: string,
  line?: number,
  col?: number,
  error?: Error,
): boolean {
  const message = typeof msg === 'string' ? msg : String(msg)
  const stack = error?.stack ?? `${url ?? 'unknown'}:${line ?? '?'}:${col ?? '?'}`
  reportError('js_error', message, stack)
  return false // Don't suppress default error behavior
}

// Global handler for unhandled promise rejections
function handleUnhandledRejection(event: PromiseRejectionEvent): void {
  const reason = event.reason
  const message = reason instanceof Error ? reason.message : String(reason)
  const stack = reason instanceof Error ? (reason.stack ?? '') : ''
  reportError('unhandled_rejection', message, stack)
}

export const errorLoggerPlugin = {
  install(app: App): void {
    // Register Vue component error handler
    app.config.errorHandler = (err: unknown, _instance, info: string): void => {
      const message = err instanceof Error ? err.message : String(err)
      const stack = err instanceof Error ? (err.stack ?? '') : ''
      const context = `Vue error in ${info}`
      reportError('vue_error', `${context}: ${message}`, stack)
    }

    // Register global error handlers
    window.onerror = handleWindowError
    window.addEventListener('unhandledrejection', handleUnhandledRejection)
  },
}
