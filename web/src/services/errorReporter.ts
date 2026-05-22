type LogLevel = 'js_error' | 'unhandled_rejection' | 'vue_error' | 'network_error' | 'debug' | 'info'

interface ErrorReport {
  timestamp: string
  type: LogLevel
  message: string
  stack: string
  url?: string
  userAgent?: string
  page?: string
}

// Duplicate detection: key = hash of message + stack, value = last reported timestamp
const reportedErrors = new Map<string, number>()
const DEDUPE_WINDOW_MS = 30_000

let isReporting = false // Prevent infinite loops from error reporter itself

function dedupeKey(report: ErrorReport): string {
  return `${report.type}:${report.message}:${report.stack?.slice(0, 100) ?? ''}`
}

function shouldReport(report: ErrorReport): boolean {
  const key = dedupeKey(report)
  const now = Date.now()
  const lastReported = reportedErrors.get(key)
  if (lastReported !== undefined && now - lastReported < DEDUPE_WINDOW_MS) {
    return false
  }
  reportedErrors.set(key, now)
  // Clean old entries
  for (const [k, ts] of reportedErrors) {
    if (now - ts > DEDUPE_WINDOW_MS) reportedErrors.delete(k)
  }
  return true
}

interface ErrorPayload {
  type: LogLevel
  message: string
  stack?: string
}

function buildReport(report: ErrorPayload): ErrorReport {
  return {
    timestamp: new Date().toISOString(),
    type: report.type,
    message: report.message,
    stack: report.stack ?? '',
    url: window.location.pathname,
    userAgent: navigator.userAgent,
    page: inferPageName(),
  }
}

function inferPageName(): string {
  const path = window.location.pathname
  if (!path || path === '/') return 'Unknown'
  // Extract the last meaningful segment
  const segments = path.split('/').filter(Boolean)
  return segments.length > 0 ? segments[segments.length - 1]! : 'Unknown'
}

async function sendReport(report: ErrorReport): Promise<void> {
  if (isReporting) return // Prevent recursive error reporting
  isReporting = true

  try {
    await fetch('/api/log', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(report),
    })
  } catch {
    // Silently ignore — never retry to avoid infinite loops
  } finally {
    isReporting = false
  }
}

export function reportError(type: LogLevel, message: string, stack?: string): void {
  const payload = buildReport({ type, message, stack })
  if (!shouldReport(payload)) return
  sendReport(payload)
}

export function debugLog(message: string, context?: Record<string, unknown>): void {
  const payload = buildReport({
    type: 'debug',
    message: context ? `${message} | context: ${JSON.stringify(context)}` : message,
  })
  sendReport(payload)
}
