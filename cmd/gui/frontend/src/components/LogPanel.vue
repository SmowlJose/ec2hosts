<script setup>
import { ref, watch, nextTick } from 'vue'
import { useAppStore } from '../stores/app'

// LogPanel shows the stream of `progress` events emitted by the Go
// backend during Up/Down, plus any client-side errors. Auto-scrolls to
// the bottom when a new entry arrives.
const store = useAppStore()
const scroller = ref(null)

const fmtTime = (ts) =>
  ts.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' })

watch(
  () => store.log.length,
  async () => {
    await nextTick()
    if (scroller.value) {
      scroller.value.scrollTop = scroller.value.scrollHeight
    }
  },
)
</script>

<template>
  <section class="panel">
    <header>
      <span>Log</span>
      <a v-if="store.log.length" @click="store.clearLog()">clear</a>
    </header>
    <div ref="scroller" class="body">
      <div
        v-for="(entry, idx) in store.log"
        :key="idx"
        :class="['line', entry.level]"
      >
        <span class="ts">{{ fmtTime(entry.ts) }}</span>
        <span class="msg">{{ entry.msg }}</span>
      </div>
      <div v-if="!store.log.length" class="empty">no activity yet</div>
    </div>
  </section>
</template>

<style scoped>
.panel {
  display: flex;
  flex-direction: column;
  border: 1px solid var(--border);
  border-radius: 8px;
  background: #0f172a;
  color: #cbd5e1;
  max-height: 180px;
}
header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 0.4rem 0.75rem;
  background: #1e293b;
  color: #94a3b8;
  font-size: 0.75em;
  text-transform: uppercase;
  letter-spacing: 0.05em;
  border-top-left-radius: 8px;
  border-top-right-radius: 8px;
}
header a { color: #60a5fa; }
.body {
  flex: 1;
  overflow-y: auto;
  padding: 0.5rem 0.75rem;
  font-family: Consolas, Menlo, monospace;
  font-size: 0.85em;
  line-height: 1.5;
}
.line { display: flex; gap: 0.75rem; }
.line.error .msg { color: #fca5a5; }
.ts { color: #64748b; flex-shrink: 0; }
.msg { white-space: pre-wrap; word-break: break-word; }
.empty { color: #475569; font-style: italic; }
</style>
