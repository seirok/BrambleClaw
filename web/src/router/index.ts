import { createRouter, createWebHistory } from 'vue-router'
import ChatView from '@/views/ChatView.vue'
import AdminView from '@/views/AdminView.vue'
import MonitoringView from '@/views/MonitoringView.vue'

const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/', component: ChatView },
    { path: '/admin', component: AdminView },
    { path: '/monitoring', component: MonitoringView },
  ],
})

export default router
