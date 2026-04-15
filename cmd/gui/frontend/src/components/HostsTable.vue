<script setup>
// Read-only view of the hosts declared in config.yaml and the target
// each one resolves to. To switch a host to a different target, the
// user edits config.yaml (v1 scope — no inline switch).
defineProps({
  items: { type: Array, default: () => [] },
})
</script>

<template>
  <section class="wrap">
    <table>
      <thead>
        <tr>
          <th>Host</th>
          <th>Target</th>
          <th>IP</th>
        </tr>
      </thead>
      <tbody>
        <tr v-if="!items.length">
          <td colspan="3" class="empty">no hosts configured</td>
        </tr>
        <tr v-for="row in items" :key="row.host">
          <td class="mono">{{ row.host }}</td>
          <td>
            <span class="pill">{{ row.target }}</span>
          </td>
          <td class="mono muted">{{ row.ip || '—' }}</td>
        </tr>
      </tbody>
    </table>
  </section>
</template>

<style scoped>
.wrap {
  flex: 1;
  overflow: auto;
  border: 1px solid var(--border);
  border-radius: 8px;
  background: #fff;
}
table { width: 100%; border-collapse: collapse; font-size: 0.92em; }
thead th {
  position: sticky;
  top: 0;
  background: var(--panel);
  text-align: left;
  padding: 0.55rem 0.85rem;
  border-bottom: 1px solid var(--border);
  font-weight: 600;
  color: var(--muted);
  text-transform: uppercase;
  font-size: 0.78em;
  letter-spacing: 0.04em;
}
tbody td {
  padding: 0.5rem 0.85rem;
  border-bottom: 1px solid var(--border);
}
tbody tr:last-child td { border-bottom: none; }
.empty { color: var(--muted); font-style: italic; text-align: center; padding: 1.5rem; }
.mono { font-family: Consolas, Menlo, monospace; }
.muted { color: var(--muted); }
.pill {
  display: inline-block;
  background: var(--panel);
  border: 1px solid var(--border);
  padding: 0.1em 0.55em;
  border-radius: 999px;
  font-family: Consolas, Menlo, monospace;
  font-size: 0.85em;
}
</style>
