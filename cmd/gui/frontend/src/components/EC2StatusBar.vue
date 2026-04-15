<script setup>
// EC2StatusBar renders one badge per instance referenced by config.yaml.
// Passive — no actions here, just the current state.
defineProps({
  items: { type: Array, default: () => [] },
})

const stateColor = (s) => {
  switch (s) {
    case 'running':       return 'success'
    case 'pending':       return 'warn'
    case 'stopping':      return 'warn'
    case 'shutting-down': return 'warn'
    case 'stopped':       return 'muted'
    case 'terminated':    return 'danger'
    case 'error':         return 'danger'
    default:              return 'muted'
  }
}
</script>

<template>
  <section class="bar">
    <div v-if="!items.length" class="empty">
      no EC2 instances referenced by this config
    </div>
    <div v-for="item in items" :key="item.instanceId" class="card">
      <div class="row">
        <span class="id">{{ item.instanceId }}</span>
        <span :class="['badge', stateColor(item.state)]">{{ item.state || 'unknown' }}</span>
      </div>
      <div class="ip">
        {{ item.publicIp || '— no public IP —' }}
      </div>
    </div>
  </section>
</template>

<style scoped>
.bar {
  display: flex;
  flex-wrap: wrap;
  gap: 0.75rem;
}
.empty {
  color: var(--muted);
  font-style: italic;
  padding: 0.5rem 0.75rem;
}
.card {
  border: 1px solid var(--border);
  border-radius: 8px;
  padding: 0.75rem 1rem;
  min-width: 240px;
  background: #fff;
}
.row {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 0.4rem;
}
.id { font-family: Consolas, Menlo, monospace; font-size: 0.9em; }

.badge {
  font-size: 0.75em;
  font-weight: 600;
  text-transform: uppercase;
  padding: 0.15em 0.6em;
  border-radius: 999px;
  letter-spacing: 0.03em;
}
.badge.success { background: #dcfce7; color: var(--success); }
.badge.warn    { background: #fef3c7; color: var(--warn); }
.badge.danger  { background: #fee2e2; color: var(--danger); }
.badge.muted   { background: #e2e8f0; color: var(--muted); }

.ip { color: var(--muted); font-family: Consolas, Menlo, monospace; }
</style>
