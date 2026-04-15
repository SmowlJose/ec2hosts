<script setup>
/*
 * EC2StatusBar — each instance rendered as a "cockpit card" with a big
 * serif index on the left, status light, state label, instance ID and
 * public IP. Deliberately information-dense but typographically quiet
 * so a five-second glance tells the operator whether everything's green.
 */
defineProps({
  items: { type: Array, default: () => [] },
})

// Map the AWS state-machine vocabulary to our three-tier visual system.
// Pending/stopping/shutting-down/rebooting share the amber "transitional"
// lane so the UI pulses the same way regardless of which direction the
// transition is heading.
const stateTier = (s) => {
  switch (s) {
    case 'running':                                       return 'up'
    case 'pending': case 'stopping':
    case 'shutting-down': case 'rebooting':               return 'transit'
    case 'stopped':                                       return 'down'
    case 'terminated': case 'error':                      return 'bad'
    default:                                              return 'down'
  }
}
</script>

<template>
  <div class="bar">
    <div v-if="!items.length" class="empty">
      <span class="label">// no instances declared in config.yaml</span>
    </div>

    <article
      v-for="(item, i) in items"
      :key="item.instanceId"
      class="card"
      :class="stateTier(item.state)"
      :style="{ animationDelay: `${i * 60}ms` }"
    >
      <!-- The big serif numeral is half-decorative, half-functional:
           it gives the operator an "index" they can refer to ("card 2
           is pending") and carries the editorial voice of the app. -->
      <div class="index">{{ String(i + 1).padStart(2, '0') }}</div>

      <div class="body">
        <header class="row">
          <span class="dot" aria-hidden="true" />
          <span class="state">{{ item.state || 'unknown' }}</span>
          <span class="sep" aria-hidden="true">·</span>
          <span class="id mono">{{ item.instanceId }}</span>
        </header>

        <div class="ip mono numeric" :class="{ absent: !item.publicIp }">
          {{ item.publicIp || '— · — · — · —' }}
        </div>
      </div>
    </article>
  </div>
</template>

<style scoped>
.bar {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(280px, 1fr));
  gap: 0.75rem;
}

.empty {
  color: var(--ink-3);
  padding: 0.9rem 0;
  font-size: 12px;
}

/* ——— Card ———————————————————————————————————————————————— */

.card {
  display: grid;
  grid-template-columns: auto 1fr;
  align-items: center;
  gap: 1rem;
  padding: 0.9rem 1.1rem;
  border: 1px solid var(--line);
  border-radius: var(--radius);
  background:
    linear-gradient(180deg, rgba(255, 255, 255, 0.015), transparent 70%),
    var(--surface-1);
  position: relative;
  overflow: hidden;
  animation: enter 0.45s var(--ease-out) backwards;
  transition: border-color 0.2s var(--ease), transform 0.2s var(--ease);
}
.card:hover {
  border-color: var(--line-strong);
  transform: translateY(-1px);
}

/* Left-edge state stripe — hairline in the theme color. Because it's
   1px and colored, it reads like a panel-meter indicator more than a
   decorative flourish. */
.card::before {
  content: '';
  position: absolute;
  left: 0; top: 0; bottom: 0;
  width: 2px;
  background: var(--line);
  transition: background 0.2s var(--ease);
}
.card.up::before     { background: var(--lime); }
.card.transit::before { background: var(--amber); }
.card.bad::before    { background: var(--ember); }

/* ——— Numeral ———————————————————————————————————————————— */

.index {
  font-family: var(--font-display);
  font-style: italic;
  font-variation-settings: 'opsz' 144, 'SOFT' 100, 'WONK' 1;
  font-weight: 300;
  font-size: 42px;
  line-height: 0.9;
  color: var(--ink-4);
  letter-spacing: -0.04em;
  min-width: 1.4em;
  text-align: right;
  transition: color 0.2s var(--ease);
}
.card.up      .index { color: var(--lime-dim); }
.card.transit .index { color: var(--amber-dim); }
.card.bad     .index { color: var(--ember); opacity: 0.55; }

/* ——— Body ———————————————————————————————————————————————— */

.body {
  display: flex;
  flex-direction: column;
  gap: 0.35rem;
  min-width: 0;
}

.row {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  min-width: 0;
}

/* Status dot — hollow circle with a fill in the theme color. Running
   gets a soft glow so the eye tracks it across the grid; transitional
   states pulse; dead states just sit there. */
.dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: var(--ink-4);
  flex-shrink: 0;
}
.card.up .dot {
  background: var(--lime);
  box-shadow: 0 0 10px var(--lime-glow), 0 0 0 3px rgba(185, 220, 91, 0.12);
}
.card.transit .dot {
  background: var(--amber);
  box-shadow: 0 0 10px var(--amber-hot);
  animation: pulse 1.4s var(--ease) infinite;
}
.card.bad .dot {
  background: var(--ember);
  box-shadow: 0 0 8px var(--ember);
}

.state {
  font-family: var(--font-mono);
  font-size: 10.5px;
  text-transform: uppercase;
  letter-spacing: 0.14em;
  color: var(--ink-2);
}
.card.up      .state { color: var(--lime-glow); }
.card.transit .state { color: var(--amber-hot); }
.card.bad     .state { color: var(--ember-hot); }

.sep { color: var(--ink-4); }

.id {
  font-size: 11.5px;
  color: var(--ink-3);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.ip {
  font-size: 18px;
  color: var(--ink);
  letter-spacing: 0.01em;
  font-weight: 500;
}
.ip.absent {
  color: var(--ink-4);
  font-weight: 300;
}
</style>
