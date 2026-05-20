<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { api } from '@/composables/useApi'
import type { Agent, ConfigResponse } from '@/types/api'

const activeTab = ref('config')
const configJson = ref('')
const agents = ref<Agent[]>([])
const loading = ref(false)
const error = ref('')

async function loadConfig() {
  loading.value = true
  error.value = ''
  try {
    const config = await api.get<ConfigResponse>('/api/admin/config')
    configJson.value = JSON.stringify(config, null, 2)
  } catch (err) {
    error.value = (err as Error).message
  } finally {
    loading.value = false
  }
}

async function saveConfig() {
  try {
    const config = JSON.parse(configJson.value) as ConfigResponse
    await api.put('/api/admin/config', config)
    window.alert('Config saved')
  } catch (err) {
    window.alert(`Invalid JSON: ${(err as Error).message}`)
  }
}

async function loadAgents() {
  try {
    agents.value = await api.get<Agent[]>('/api/admin/agents')
  } catch (err) {
    error.value = (err as Error).message
  }
}

function setActiveTab(tab: string) {
  activeTab.value = tab
}

function resetAgent(name: string) {
  window.alert('Reset agent ' + name)
}

onMounted(() => {
  loadConfig()
  loadAgents()
})
</script>

<template>
  <div class="admin-container">
    <h1>Admin</h1>

    <div class="admin-tabs">
      <div
        class="admin-tab"
        :class="{ active: activeTab === 'config' }"
        @click="setActiveTab('config')"
      >
        Configuration
      </div>
      <div
        class="admin-tab"
        :class="{ active: activeTab === 'agents' }"
        @click="setActiveTab('agents')"
      >
        Agents
      </div>
      <div
        class="admin-tab"
        :class="{ active: activeTab === 'audit' }"
        @click="setActiveTab('audit')"
      >
        Audit Log
      </div>
    </div>

    <div v-if="error" class="error-banner" style="color: var(--color-error); margin-bottom: 16px">
      Error: {{ error }}
    </div>

    <div id="admin-content">
      <!-- Config Tab -->
      <div v-show="activeTab === 'config'">
        <div v-if="loading">Loading...</div>
        <div v-else class="config-editor">
          <textarea id="config-json" v-model="configJson"></textarea>
          <div class="config-actions">
            <button class="btn-save" @click="saveConfig">Save</button>
            <button class="btn-refresh" @click="loadConfig">Refresh</button>
          </div>
        </div>
      </div>

      <!-- Agents Tab -->
      <div v-show="activeTab === 'agents'">
        <div v-if="agents.length === 0 && !loading" style="color: var(--color-text-secondary)">
          No agents configured
        </div>
        <div v-else class="agent-list">
          <div v-for="agent in agents" :key="agent.name" class="agent-card">
            <h3>{{ agent.name }}</h3>
            <div class="agent-model">{{ agent.model }}</div>
            <div class="agent-tools">
              <span v-for="tool in agent.tools" :key="tool" class="tool-tag">{{ tool }}</span>
            </div>
            <div class="agent-actions">
              <button @click="resetAgent(agent.name)">Reset Session</button>
            </div>
          </div>
        </div>
      </div>

      <!-- Audit Tab -->
      <div v-show="activeTab === 'audit'">
        <table class="audit-table">
          <thead>
            <tr><th>Time</th><th>Event</th><th>Details</th></tr>
          </thead>
          <tbody>
            <tr>
              <td colspan="3" style="text-align: center; color: var(--color-text-secondary)">
                Audit log coming soon
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>
  </div>
</template>
