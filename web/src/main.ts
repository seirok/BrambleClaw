import { createApp } from 'vue'
import App from './App.vue'
import router from './router'

import './css/reset.css'
import './css/variables.css'
import './css/layout.css'
import './css/chat.css'
import './css/admin.css'
import './css/monitoring.css'

const app = createApp(App)
app.use(router)
app.mount('#app')
