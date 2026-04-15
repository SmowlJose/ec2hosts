<script setup>
import { onMounted, onBeforeUnmount, computed, ref } from 'vue'
import { useAppStore } from './stores/app'
import EC2StatusBar from './components/EC2StatusBar.vue'
import ActionButtons from './components/ActionButtons.vue'
import HostsTable from './components/HostsTable.vue'
import LogPanel from './components/LogPanel.vue'
import AWSCredentialsDialog from './components/AWSCredentialsDialog.vue'

const store = useAppStore()

const configMissing = computed(
  () => store.configInfo && !store.configInfo.found,
)

// Rotating "LIVE" indicator in the header. Pure cosmetic — it ticks
// independent of network activity, meant as a reassurance that the
// console is breathing. It only shows the dot; the "LIVE" word stays
// steady so the eye doesn't twitch during reads.
const live = ref(true)
let liveTimer = null

// Current wall-clock shown monospace next to the brand. Updates once
// per second. Kept in the App shell (not a sub-component) because
// it's part of the chrome, not the data.
const now = ref(new Date())
let clockTimer = null
const clockStr = computed(() =>
  now.value.toLocaleTimeString([], {
    hour: '2-digit', minute: '2-digit', second: '2-digit', hour12: false,
  }),
)

// Compact config path — show just the final segment and ellipsize the
// middle so the header stays single-line on narrow windows. Full path
// still shown on hover via title="".
const compactPath = computed(() => {
  const p = store.configInfo?.path || ''
  if (!p) return ''
  const norm = p.replaceAll('\\', '/').split('/')
  if (norm.length <= 3) return p
  return [norm[0], '…', norm[norm.length - 2], norm[norm.length - 1]].join('/')
})

onMounted(async () => {
  store.subscribeEvents()
  await store.loadConfigInfo()
  if (!configMissing.value) {
    await store.refresh()
    store.startPolling()
  }
  clockTimer = setInterval(() => { now.value = new Date() }, 1000)
  liveTimer  = setInterval(() => { live.value = !live.value }, 1100)
})

onBeforeUnmount(() => {
  store.stopPolling()
  if (clockTimer) clearInterval(clockTimer)
  if (liveTimer)  clearInterval(liveTimer)
})
</script>

<template>
  <div class="shell">
    <!-- ——— Header —————————————————————————————————————————————— -->
    <header class="chrome">
      <div class="brand">
        <!-- The brand is the one place the serif leans in. Italic
             "ec2" paired with a heavier upright "/hosts" gives the
             whole app a byline. -->
        <h1>
          <span class="brand-ec2">ec2</span><span class="brand-slash">/</span><span class="brand-hosts">hosts</span>
        </h1>
        <div class="meta">
          <span class="meta-dot" :class="{ off: !live }" />
          <span class="label">live</span>
          <span class="sep" aria-hidden="true">·</span>
          <span class="label">{{ clockStr }}</span>
          <span v-if="store.configInfo?.path" class="sep" aria-hidden="true">·</span>
          <span
            v-if="store.configInfo?.path"
            class="path"
            :title="store.configInfo.path"
          >{{ compactPath }}</span>
        </div>
      </div>

      <nav class="nav">
        <a @click="store.openCredsDialog">
          <span class="chev">›</span> aws credentials
        </a>
        <a @click="store.openConfigInEditor">
          <span class="chev">›</span> edit config
        </a>
        <a @click="store.openConfigFolder">
          <span class="chev">›</span> open folder
        </a>
      </nav>
    </header>

    <AWSCredentialsDialog />

    <!-- ——— Missing config — onboarding state ——————————————————— -->
    <main v-if="configMissing" class="onboarding">
      <p class="eyebrow label">no config.yaml detected</p>
      <h2 class="display">
        Point us at a <em>config.yaml</em>.
      </h2>
      <p class="onboarding-body">
        ec2hosts reads its hosts and EC2 targets from a YAML file. It looks
        next to the binary first, then at
        <code>%APPDATA%\ec2hosts\config.yaml</code>.
      </p>
      <p v-if="store.configInfo?.error" class="onboarding-err mono">
        {{ store.configInfo.error }}
      </p>
      <div class="onboarding-cta">
        <button class="primary" @click="store.openConfigFolder">
          Open config folder
        </button>
        <button @click="store.openCredsDialog">Set AWS credentials</button>
      </div>
    </main>

    <!-- ——— Main console ——————————————————————————————————————— -->
    <main v-else class="console">
      <section class="section instances">
        <header class="section-head">
          <h2 class="label">EC2 Instances</h2>
          <span class="count mono numeric">
            {{ String(store.ec2.length).padStart(2, '0') }}
          </span>
        </header>
        <EC2StatusBar :items="store.ec2" />
      </section>

      <section class="section controls">
        <header class="section-head">
          <h2 class="label">Controls</h2>
        </header>
        <ActionButtons />
      </section>

      <section class="section hosts">
        <header class="section-head">
          <h2 class="label">Hosts</h2>
          <span class="count mono numeric">
            {{ String(store.hosts.length).padStart(2, '0') }}
          </span>
        </header>
        <HostsTable :items="store.hosts" />
      </section>

      <section class="section log">
        <header class="section-head">
          <h2 class="label">Log</h2>
          <a
            v-if="store.log.length"
            class="clear-link"
            @click="store.clearLog()"
          >clear</a>
        </header>
        <LogPanel />
      </section>
    </main>
  </div>
</template>

<style scoped>
/* Top-level shell — full height column. A 1px hairline under the header
   plus a wider top/bottom gutter gives everything room to breathe. */
.shell {
  display: flex;
  flex-direction: column;
  height: 100%;
}

/* ——— Header ———————————————————————————————————————————————— */
.chrome {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 1.1rem 1.5rem 1rem;
  border-bottom: 1px solid var(--line);
  background:
    linear-gradient(180deg, rgba(255, 255, 255, 0.02), transparent 60%),
    var(--surface);
  flex-shrink: 0;
}

.brand { display: flex; flex-direction: column; gap: 0.35rem; }
h1 {
  margin: 0;
  line-height: 1;
  font-weight: 400;
  font-size: 26px;
  letter-spacing: -0.02em;
}
.brand-ec2 {
  font-family: var(--font-display);
  font-style: italic;
  font-variation-settings: 'opsz' 144, 'SOFT' 100;
  color: var(--amber);
}
.brand-slash {
  font-family: var(--font-mono);
  font-weight: 300;
  color: var(--ink-4);
  margin: 0 0.12em;
}
.brand-hosts {
  font-family: var(--font-display);
  font-weight: 600;
  font-variation-settings: 'opsz' 144;
  color: var(--ink);
}

.meta {
  display: flex;
  align-items: center;
  gap: 0.55rem;
  font-size: 10.5px;
  color: var(--ink-3);
}
.meta-dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: var(--lime);
  box-shadow: 0 0 10px var(--lime-glow);
  transition: opacity 0.35s var(--ease), transform 0.35s var(--ease);
}
.meta-dot.off { opacity: 0.35; transform: scale(0.8); }
.sep { color: var(--ink-4); }
.path {
  font-family: var(--font-mono);
  color: var(--ink-3);
  max-width: 300px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.nav {
  display: flex;
  gap: 1.1rem;
  align-items: center;
  font-size: 11px;
  text-transform: uppercase;
  letter-spacing: 0.08em;
}
.nav a {
  color: var(--ink-3);
  padding: 0.3rem 0;
  display: inline-flex;
  align-items: center;
  gap: 0.4em;
  border-bottom: none;
}
.nav a:hover { color: var(--amber); }
.nav .chev {
  font-family: var(--font-mono);
  color: var(--ink-4);
  transition: color 0.15s var(--ease), transform 0.2s var(--ease);
}
.nav a:hover .chev {
  color: var(--amber);
  transform: translateX(2px);
}

/* ——— Main console grid —————————————————————————————————————— */

.console {
  flex: 1;
  overflow: hidden;
  padding: 1.5rem 1.5rem 1.25rem;
  display: grid;
  grid-template-rows: auto auto 1fr auto;
  gap: 1.5rem;
  animation: enter 0.5s var(--ease-out);
}

.section { display: flex; flex-direction: column; gap: 0.8rem; min-height: 0; }

.section-head {
  display: flex;
  align-items: baseline;
  gap: 0.75rem;
}
.section-head h2 { margin: 0; }
.section-head .count {
  color: var(--ink-4);
  font-size: 11px;
  font-weight: 500;
  letter-spacing: 0.05em;
}
.section-head .clear-link {
  margin-left: auto;
  font-size: 10.5px;
  letter-spacing: 0.12em;
  text-transform: uppercase;
  color: var(--ink-3);
}

.section.hosts { min-height: 0; flex: 1; }
.section.hosts :deep(.wrap) { min-height: 0; }

/* ——— Onboarding state ——————————————————————————————————————— */

.onboarding {
  flex: 1;
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  justify-content: center;
  padding: 3rem 4rem;
  max-width: 600px;
  gap: 1.1rem;
  animation: enter 0.55s var(--ease-out);
}
.onboarding .eyebrow {
  margin: 0;
  color: var(--amber);
}
.onboarding h2 {
  margin: 0;
  font-size: 44px;
  line-height: 1.05;
  font-weight: 400;
  color: var(--ink);
  font-variation-settings: 'opsz' 144, 'SOFT' 40;
  letter-spacing: -0.02em;
}
.onboarding h2 em {
  font-style: italic;
  font-variation-settings: 'opsz' 144, 'SOFT' 100;
  color: var(--amber);
}
.onboarding-body {
  margin: 0;
  color: var(--ink-2);
  font-size: 14px;
  line-height: 1.55;
  max-width: 50ch;
}
.onboarding-body code {
  font-family: var(--font-mono);
  color: var(--ink);
  background: var(--surface-2);
  padding: 0.08em 0.4em;
  border-radius: var(--radius-sm);
  font-size: 12.5px;
}
.onboarding-err {
  color: var(--ember);
  background: var(--ember-wash);
  border: 1px solid rgba(232, 100, 69, 0.3);
  border-left: 3px solid var(--ember);
  padding: 0.7rem 0.9rem;
  border-radius: var(--radius);
  font-size: 12px;
  word-break: break-word;
  max-width: 60ch;
}
.onboarding-cta {
  display: flex;
  gap: 0.7rem;
  margin-top: 0.5rem;
}
</style>
