export interface Agent {
  name: string
  description?: string
  model: string
  status: string
  tools?: string[]
}

export interface ChatEvent {
  content?: string
  error?: string
  type?: string
}

export interface MetricsSummary {
  total_requests: number
  avg_latency_ms: number
  error_rate: number
  total_errors: number
  total_tokens: number
}

export interface EndpointStats {
  requests: number
  errors: number
  error_rate: number
  avg_latency: number
}

export type MetricsChannels = Record<string, EndpointStats>

export interface HealthStatus {
  status: string
  time: string
}

export interface ConfigResponse {
  [key: string]: unknown
}
