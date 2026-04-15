<script setup>
import { ref, watch, nextTick } from 'vue'
import { useAppStore } from '../stores/app'

/*
 * LogPanel — reads like a real terminal log. Each line carries a small
 * caret (▸ for info, ! for errors), a dim timestamp, and the message.
 * Auto-scrolls on new entries but respects a user who has scrolled up
 * to read history: we only snap to bottom when they were already there.
 */
const store = useAppStore()
const scroller = ref(null)
const pinnedToBottom = ref(true)

const fmtTime = (ts) =>
  ts.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit', hour12: false })

function onScroll() {
  const el = scroller.value
  if (!el) return
  // Treat "within 8px of the bottom" as pinned, so tiny sub-pixel
  // rounding from Wails doesn't un-pin the user by accident.
  pinnedToBottom.value =
    el.scrollTop + el.clientHeight >= el.scrollHeight - 8
}

watch(
  () => store.log.length,
  async () => {
    await nextTick()
    if (pinnedToBottom.value && scroller.value) {
      scroller.value.scrollTop = scroller.value.scrollHeight
    }
  },
)
</script>

<template>
  <div
    ref="scroller"
    class="panel"
    :data-empty="!store.log.length"
    @scroll="onScroll"
  >
    <div
      v-for="(entry, idx) in store.log"
      :key="idx"
      class="line"
      :class="entry.level"
    >
      <span class="caret" aria-hidden="true">{{ entry.level === 'error' ? '!' : '▸' }}</span>
      <span class="ts mono">{{ fmtTime(entry.ts) }}</span>
      <span class="msg">{{ entry.msg }}</span>
    </div>
    <div v-if="!store.log.length" class="empty">
      <span class="caret" aria-hidden="true">▸</span>
      <span class="label">waiting for input</span>
      <span class="cursor" aria-hidden="true">_</span>
    </div>
  </div>
</template>

<style scoped>
.panel {
  max-height: 180px;
  overflow-y: auto;
  padding: 0.65rem 0.9rem;
  font-family: var(--font-mono);
  font-size: 12px;
  line-height: 1.55;
  border: 1px solid var(--line);
  border-radius: var(--radius);
  background:
    linear-gradient(180deg, rgba(0, 0, 0, 0.25), transparent 30%),
    var(--surface);
  /* Terminal scanline ambience — only on the log panel, not anywhere
     else. Keeps the association "log = terminal" without polluting
     the rest of the app. */
  background-image:
    linear-gradient(180deg, rgba(0, 0, 0, 0.25), transparent 30%),
    repeating-linear-gradient(
      0deg,
      rgba(255, 255, 255, 0.012) 0px,
      rgba(255, 255, 255, 0.012) 1px,
      transparent 1px,
      transparent 3px
    );
}

.line {
  display: grid;
  grid-template-columns: 14px 72px 1fr;
  gap: 0.6rem;
  align-items: baseline;
  padding: 1px 0;
  animation: enter 0.25s var(--ease-out);
}

.caret {
  color: var(--ink-4);
  font-family: var(--font-mono);
  font-size: 11px;
  line-height: 1.55;
  text-align: center;
}
.line.info  .caret { color: var(--amber-dim); }
.line.error .caret {
  color: var(--ember);
  font-weight: 700;
}

.ts {
  color: var(--ink-4);
  font-size: 11px;
  font-variant-numeric: tabular-nums;
}

.msg {
  color: var(--ink-2);
  white-space: pre-wrap;
  word-break: break-word;
}
.line.error .msg { color: var(--ember-hot); }

.empty {
  display: flex;
  align-items: center;
  gap: 0.6rem;
  color: var(--ink-4);
  font-style: italic;
}
.empty .caret { color: var(--ink-4); }
.empty .label { text-transform: none; letter-spacing: 0.06em; font-size: 11.5px; }
.cursor {
  color: var(--amber);
  animation: blink 1s steps(2, end) infinite;
}
@keyframes blink {
  from, to { opacity: 0; }
  50%      { opacity: 1; }
}
</style>
