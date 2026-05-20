<script setup lang="ts">
import { ref, onMounted, onUnmounted } from 'vue'
import { api } from '@/composables/useApi'
import type { MetricsSummary, MetricsChannels } from '@/types/api'

const metricsSummary = ref<MetricsSummary | null>(null)
const channelStats = ref<MetricsChannels | null>(null)
const apiOnline = ref(true)
let refreshInterval: ReturnType<typeof setInterval> | null = null

async function loadMetrics() {
  try {
    const summary = await api.get<MetricsSummary>('/api/metrics/summary')
    metricsSummary.value = summary
    apiOnline.value = true
  } catch {
    apiOnline.value = false
  }

  try {
    const channels = await api.get<MetricsChannels>('/api/metrics/channels')
    channelStats.value = channels
  } catch {
    // Metrics may require API key
  }
}

const channelEntries = (stats: MetricsChannels) => Object.entries(stats)

onMounted(() => {
  loadMetrics()
  refreshInterval = setInterval(loadMetrics, 30000)
})

onUnmounted(() => {
  if (refreshInterval) {
    clearInterval(refreshInterval)
    refreshInterval = null
  }
})
</script>

<template>
  <div class="monitoring-container">
    <div style="display: flex; align-items: center; justify-content: space-between">
      <h1>Monitoring</h1>
      <span class="auto-refresh">Auto-refresh: every 30s</span>
    </div>

    <div id="metric-cards" class="metric-cards">
      <template v-if="metricsSummary">
        <div class="metric-card">
          <div class="metric-label">Total Requests</div>
          <div class="metric-value">{{ metricsSummary.total_requests || 0 }}</div>
        </div>
        <div class="metric-card">
          <div class="metric-label">Avg Latency</div>
          <div class="metric-value">{{ metricsSummary.avg_latency_ms || 0 }}ms</div>
        </div>
        <div class="metric-card">
          <div class="metric-label">Error Rate</div>
          <div class="metric-value">{{ (metricsSummary.error_rate || 0).toFixed(1) }}%</div>
          <div class="metric-sub">{{ metricsSummary.total_errors || 0 }} errors</div>
        </div>
        <div class="metric-card">
          <div class="metric-label">Tokens Used</div>
          <div class="metric-value">{{ metricsSummary.total_tokens || 0 }}</div>
        </div>
      </template>
      <template v-else>
        <div class="metric-card">
          <div class="metric-label">Loading...</div>
        </div>
      </template>
    </div>

    <div class="health-section">
      <h2>System Health</h2>
      <div id="health-grid" class="health-grid">
        <div class="health-item">
          <div class="health-dot" :class="apiOnline ? 'healthy' : 'error'"></div>
          <span class="health-name">API Server</span>
          <span class="health-status">{{ apiOnline ? 'Online' : 'Offline' }}</span>
        </div>
      </div>
    </div>

    <div class="health-section">
      <h2>Endpoint Stats</h2>
      <div id="endpoint-stats">
        <template v-if="channelStats && Object.entries(channelStats).length > 0">
          <table class="stats-table">
            <thead>
              <tr>
                <th>Endpoint</th>
                <th>Requests</th>
                <th>Errors</th>
                <th>Error Rate</th>
                <th>Avg Latency</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="[path, stats] in channelEntries(channelStats)" :key="path">
                <td><code>{{ path }}</code></td>
                <td>{{ stats.requests }}</td>
                <td>{{ stats.errors }}</td>
                <td :class="{ 'error-rate-high': stats.error_rate > 10 }">
                  {{ stats.error_rate.toFixed(1) }}%
                </td>
                <td>{{ stats.avg_latency }}ms</td>
              </tr>
            </tbody>
          </table>
        </template>
        <template v-else>
          <p style="color: var(--color-text-secondary); font-size: 0.9rem">
            No endpoint data yet
          </p>
        </template>
      </div>
    </div>
  </div>
</template>
