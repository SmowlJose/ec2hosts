<script setup>
/*
 * HostsTable — editorial, dense, monospace table. The columns are sized
 * so the host name is always the widest lane (because that's what the
 * eye searches for) and the IP is right-aligned with tabular numerals
 * so they visually stack across rows.
 */
defineProps({
  items: { type: Array, default: () => [] },
})
</script>

<template>
  <div class="wrap">
    <table>
      <thead>
        <tr>
          <th class="col-host">Host</th>
          <th class="col-target">Target</th>
          <th class="col-ip">IP</th>
        </tr>
      </thead>
      <tbody>
        <tr v-if="!items.length">
          <td colspan="3" class="empty">
            <span class="label">// no hosts declared</span>
          </td>
        </tr>
        <tr
          v-for="(row, i) in items"
          :key="row.host"
          :style="{ animationDelay: `${Math.min(i, 20) * 18}ms` }"
        >
          <td class="host mono">
            <span class="host-glyph" aria-hidden="true">↳</span>
            <span>{{ row.host }}</span>
          </td>
          <td class="target">
            <span class="pill mono">{{ row.target }}</span>
          </td>
          <td class="ip mono numeric" :class="{ absent: !row.ip }">
            {{ row.ip || '—' }}
          </td>
        </tr>
      </tbody>
    </table>
  </div>
</template>

<style scoped>
.wrap {
  flex: 1;
  min-height: 0;
  overflow: auto;
  border: 1px solid var(--line);
  border-radius: var(--radius);
  background: var(--surface-1);
}

table {
  width: 100%;
  border-collapse: separate;
  border-spacing: 0;
  font-size: 12.5px;
}

thead th {
  position: sticky;
  top: 0;
  z-index: 1;
  text-align: left;
  padding: 0.65rem 1rem;
  background: var(--surface-1);
  border-bottom: 1px solid var(--line);
  font-family: var(--font-mono);
  font-weight: 500;
  color: var(--ink-3);
  text-transform: uppercase;
  font-size: 10.5px;
  letter-spacing: 0.16em;
}
th.col-host   { width: auto; }
th.col-target { width: 14%; min-width: 100px; }
th.col-ip     { width: 24%; min-width: 160px; text-align: right; }

tbody tr {
  transition: background 0.12s var(--ease);
  animation: enter 0.35s var(--ease-out) backwards;
}
/* Subtle zebra — only the odd rows. Barely perceptible but makes long
   tables scan-able without shouting "zebra". */
tbody tr:nth-child(even) { background: rgba(255, 255, 255, 0.012); }
tbody tr:hover { background: var(--amber-wash); }
tbody tr:hover .host-glyph { color: var(--amber); }

tbody td {
  padding: 0.55rem 1rem;
  border-bottom: 1px solid rgba(42, 38, 32, 0.55);
  vertical-align: middle;
}
tbody tr:last-child td { border-bottom: none; }

/* ——— Cells ———————————————————————————————————————————————— */

.host {
  display: flex;
  align-items: center;
  gap: 0.55rem;
  color: var(--ink);
  font-size: 12.5px;
}
.host-glyph {
  color: var(--ink-4);
  font-family: var(--font-mono);
  transition: color 0.15s var(--ease);
}

.pill {
  display: inline-flex;
  align-items: center;
  padding: 0.12em 0.55em;
  font-size: 10.5px;
  letter-spacing: 0.1em;
  text-transform: uppercase;
  color: var(--ink-2);
  background: var(--surface-2);
  border: 1px solid var(--line);
  border-radius: 2px;
}

.ip {
  text-align: right;
  color: var(--ink);
  font-size: 12.5px;
  letter-spacing: 0.01em;
}
.ip.absent { color: var(--ink-4); }

.empty {
  color: var(--ink-3);
  text-align: center;
  padding: 2rem 1rem !important;
}
</style>
