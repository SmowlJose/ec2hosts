<script setup>
import { onMounted, onBeforeUnmount } from 'vue'
import { useAppStore } from '../stores/app'

const store = useAppStore()

// Keyboard shortcuts live here so the chips on the buttons are the
// source of truth. Every global shortcut is also surfaced visually,
// which is the one UX investment that pays dividends forever.
function onKey(e) {
  // Ignore when the user is typing in an input/textarea/dialog.
  const t = e.target
  if (t && (t.tagName === 'INPUT' || t.tagName === 'TEXTAREA' || t.isContentEditable)) return
  if (store.showCredsDialog) return

  const mod = e.ctrlKey || e.metaKey
  if (mod && e.key === 'Enter') { e.preventDefault(); store.up() }
  else if (mod && e.key === '.')  { e.preventDefault(); store.down() }
  else if (e.key === 'r' && !mod) { e.preventDefault(); store.refresh() }
}
onMounted(() => window.addEventListener('keydown', onKey))
onBeforeUnmount(() => window.removeEventListener('keydown', onKey))
</script>

<template>
  <section class="strip">
    <button
      class="primary"
      :disabled="store.busy"
      @click="store.up()"
      title="Start instances and apply hosts"
    >
      <span class="glyph" aria-hidden="true">▶</span>
      <span class="text">Start &amp; apply</span>
      <span class="kbd"><span>⌘</span><span>↵</span></span>
    </button>

    <button
      class="danger"
      :disabled="store.busy"
      @click="store.down()"
      title="Stop every EC2 instance referenced by config.yaml"
    >
      <span class="glyph" aria-hidden="true">■</span>
      <span class="text">Stop</span>
      <span class="kbd"><span>⌘</span><span>.</span></span>
    </button>

    <button
      :disabled="store.busy"
      @click="store.refresh()"
      title="Re-read EC2 state and hosts file"
    >
      <span class="glyph" aria-hidden="true">↻</span>
      <span class="text">Refresh</span>
      <span class="kbd"><span>R</span></span>
    </button>

    <!-- Spacer so the status line floats right-aligned. Using margin-left:
         auto on the status block itself keeps the group intact when the
         window gets narrow and the buttons wrap. -->
    <div class="spacer" />

    <Transition name="fade" mode="out-in">
      <div v-if="store.busy" key="busy" class="status busy">
        <span class="bar" />
        <span class="label">working…</span>
      </div>
      <div
        v-else-if="store.lastError"
        key="err"
        class="status err"
        :title="store.lastError"
      >
        <span class="glyph">!</span>
        <span class="label">error — hover for detail</span>
      </div>
      <div v-else key="idle" class="status idle">
        <span class="label">idle</span>
      </div>
    </Transition>
  </section>
</template>

<style scoped>
.strip {
  display: flex;
  flex-wrap: wrap;
  gap: 0.6rem;
  align-items: center;
}

button {
  gap: 0.55em;
  padding: 0.75em 1em;
  font-size: 11.5px;
  letter-spacing: 0.08em;
}
.glyph {
  font-size: 9px;
  line-height: 1;
  translate: 0 -1px;
  font-family: var(--font-mono);
}
.text { flex: 0 0 auto; }

.spacer { flex: 1 1 0; }

/* ——— Status readout ——————————————————————————————————— */

.status {
  display: inline-flex;
  align-items: center;
  gap: 0.5rem;
  padding: 0.35em 0.6em;
  border-radius: var(--radius);
  font-size: 10.5px;
}
.status .label {
  text-transform: uppercase;
  letter-spacing: 0.14em;
  color: var(--ink-3);
}

.status.idle .label { color: var(--ink-4); }

.status.busy {
  color: var(--amber);
  background: var(--amber-wash);
  border: 1px solid rgba(232, 162, 59, 0.25);
}
.status.busy .label { color: var(--amber); }

/* The "bar" here is a stylized activity indicator — 3 stacked lines
   that swap opacity. Subtle enough to pulse without distracting. */
.status.busy .bar {
  position: relative;
  width: 10px;
  height: 10px;
}
.status.busy .bar::before,
.status.busy .bar::after,
.status.busy .bar {
  /* fallback — overridden below */
}
.status.busy .bar {
  background:
    linear-gradient(90deg, var(--amber) 50%, transparent 50%) 0 0 / 100% 2px no-repeat,
    linear-gradient(90deg, var(--amber) 50%, transparent 50%) 0 4px / 100% 2px no-repeat,
    linear-gradient(90deg, var(--amber) 50%, transparent 50%) 0 8px / 100% 2px no-repeat;
  animation: bar-shift 0.9s steps(4) infinite;
}
@keyframes bar-shift {
  0%   { background-position: 0 0, 0 4px, 0 8px; }
  25%  { background-position: 2px 0, 4px 4px, 1px 8px; }
  50%  { background-position: 4px 0, 1px 4px, 3px 8px; }
  75%  { background-position: 6px 0, 3px 4px, 5px 8px; }
  100% { background-position: 8px 0, 5px 4px, 7px 8px; }
}

.status.err {
  color: var(--ember);
  background: var(--ember-wash);
  border: 1px solid rgba(232, 100, 69, 0.3);
  cursor: help;
}
.status.err .label { color: var(--ember); }
.status.err .glyph {
  font-family: var(--font-mono);
  font-weight: 700;
  width: 14px;
  height: 14px;
  border: 1px solid currentColor;
  border-radius: 50%;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  font-size: 9px;
}

.fade-enter-active, .fade-leave-active {
  transition: opacity 0.2s var(--ease), transform 0.2s var(--ease);
}
.fade-enter-from { opacity: 0; transform: translateY(-2px); }
.fade-leave-to   { opacity: 0; transform: translateY(2px); }
</style>
