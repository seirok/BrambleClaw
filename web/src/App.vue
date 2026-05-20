<script setup lang="ts">
import { ref, onMounted } from 'vue'
import ParticleBackground from '@/components/ParticleBackground.vue'
import { api } from '@/composables/useApi'
api.init('')

const healthOk = ref(false)

onMounted(async () => {
  try {
    await api.get('/api/health')
    healthOk.value = true
  } catch {
    healthOk.value = false
  }
})
</script>

<template>
  <ParticleBackground />
  <div id="noise-overlay"></div>

  <div class="app">
    <!-- Sidebar -->
    <aside class="sidebar">
      <div class="sidebar-logo">
        <div class="logo-icon">⚡</div>
        <div class="logo-text">
          <span class="logo-title">NEOCLAW</span>
          <span class="logo-subtitle">AI Agent Interface</span>
        </div>
      </div>

      <nav class="sidebar-nav">
        <router-link to="/" class="nav-item" active-class="active">
          <div class="nav-icon-wrapper">
            <span class="nav-icon">💬</span>
          </div>
          <span class="nav-label">Chat</span>
          <div class="nav-indicator"></div>
        </router-link>

        <router-link to="/admin" class="nav-item" active-class="active">
          <div class="nav-icon-wrapper">
            <span class="nav-icon">⚙️</span>
          </div>
          <span class="nav-label">Admin</span>
        </router-link>

        <router-link to="/monitoring" class="nav-item" active-class="active">
          <div class="nav-icon-wrapper">
            <span class="nav-icon">📊</span>
            <div class="status-dot" :class="healthOk ? 'online' : 'offline'"></div>
          </div>
          <span class="nav-label">Monitor</span>
        </router-link>
      </nav>
    </aside>

    <!-- Main content -->
    <main class="main-content">
      <router-view />
    </main>
  </div>
</template>