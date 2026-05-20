<script setup lang="ts">
import { ref, onMounted, onBeforeUnmount } from 'vue'

const canvasRef = ref<HTMLCanvasElement | null>(null)
let animationFrame: number | null = null
let particles: Particle[] = []

interface Particle {
  x: number
  y: number
  vx: number
  vy: number
  radius: number
  opacity: number
  color: string
}

function resize() {
  const canvas = canvasRef.value
  if (!canvas) return
  canvas.width = window.innerWidth * window.devicePixelRatio
  canvas.height = window.innerHeight * window.devicePixelRatio
}

function createParticle(canvas: HTMLCanvasElement): Particle {
  return {
    x: Math.random() * canvas.width,
    y: Math.random() * canvas.height,
    vx: (Math.random() - 0.5) * 0.4,
    vy: (Math.random() - 0.5) * 0.4,
    radius: Math.random() * 1.5 + 0.5,
    opacity: Math.random() * 0.5 + 0.1,
    color: Math.random() > 0.7 ? 'rgba(0, 240, 255,' : 'rgba(108, 92, 231,',
  }
}

function animate(ctx: CanvasRenderingContext2D, canvas: HTMLCanvasElement) {
  ctx.clearRect(0, 0, canvas.width, canvas.height)

  for (const p of particles) {
    p.x += p.vx
    p.y += p.vy

    if (p.x < 0) p.x = canvas.width
    if (p.x > canvas.width) p.x = 0
    if (p.y < 0) p.y = canvas.height
    if (p.y > canvas.height) p.y = 0

    ctx.beginPath()
    ctx.arc(p.x, p.y, p.radius * window.devicePixelRatio, 0, Math.PI * 2)
    ctx.fillStyle = p.color + p.opacity + ')'
    ctx.fill()
  }

  const maxDist = 100 * window.devicePixelRatio
  for (let i = 0; i < particles.length; i++) {
    for (let j = i + 1; j < particles.length; j++) {
      const dx = particles[i]!.x - particles[j]!.x
      const dy = particles[i]!.y - particles[j]!.y
      const dist = Math.sqrt(dx * dx + dy * dy)
      if (dist < maxDist) {
        const opacity = (1 - dist / maxDist) * 0.15
        ctx.beginPath()
        ctx.moveTo(particles[i]!.x, particles[i]!.y)
        ctx.lineTo(particles[j]!.x, particles[j]!.y)
        ctx.strokeStyle = 'rgba(108, 92, 231,' + opacity + ')'
        ctx.lineWidth = 0.5 * window.devicePixelRatio
        ctx.stroke()
      }
    }
  }

  animationFrame = requestAnimationFrame(() => animate(ctx, canvas))
}

onMounted(() => {
  const canvas = canvasRef.value
  if (!canvas) return

  const ctx = canvas.getContext('2d')
  if (!ctx) return

  resize()
  window.addEventListener('resize', resize)

  const count = Math.floor((window.innerWidth * window.innerHeight) / 18000)
  for (let i = 0; i < count; i++) {
    particles.push(createParticle(canvas))
  }

  animate(ctx, canvas)
})

onBeforeUnmount(() => {
  if (animationFrame) {
    cancelAnimationFrame(animationFrame)
    animationFrame = null
  }
  particles = []
  window.removeEventListener('resize', resize)
})
</script>

<template>
  <div id="particle-canvas">
    <canvas ref="canvasRef" style="width: 100%; height: 100%"></canvas>
  </div>
</template>
