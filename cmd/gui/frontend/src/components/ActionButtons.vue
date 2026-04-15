<script setup>
import { useAppStore } from '../stores/app'

// Three actions that cover the fire-and-forget daily flow:
//   Start & apply  = `ec2hosts up`
//   Stop           = `ec2hosts down`
//   Refresh        = `ec2hosts status`
//
// Buttons are disabled while an action is running (store.busy) to
// prevent double-clicks and interleaved AWS calls.
const store = useAppStore()
</script>

<template>
  <section class="actions">
    <button class="primary" :disabled="store.busy" @click="store.up()">
      <span class="ico">▶</span>
      Start &amp; apply
    </button>
    <button class="danger" :disabled="store.busy" @click="store.down()">
      <span class="ico">■</span>
      Stop
    </button>
    <button :disabled="store.busy" @click="store.refresh()">
      <span class="ico">↻</span>
      Refresh
    </button>
    <div v-if="store.busy" class="busy">working…</div>
    <div v-else-if="store.lastError" class="err" :title="store.lastError">
      {{ store.lastError }}
    </div>
  </section>
</template>

<style scoped>
.actions {
  display: flex;
  gap: 0.75rem;
  align-items: center;
}
.ico {
  font-family: Consolas, monospace;
  font-size: 0.85em;
  margin-right: 0.35rem;
  display: inline-block;
  width: 1em;
  text-align: center;
}
.busy {
  color: var(--muted);
  font-style: italic;
  margin-left: 0.5rem;
}
.err {
  color: var(--danger);
  font-family: Consolas, Menlo, monospace;
  font-size: 0.9em;
  margin-left: 0.5rem;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  max-width: 420px;
}
</style>
