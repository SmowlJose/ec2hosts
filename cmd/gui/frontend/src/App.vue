<script setup>
import { onMounted, onBeforeUnmount, computed } from 'vue'
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

onMounted(async () => {
  store.subscribeEvents()
  await store.loadConfigInfo()
  if (!configMissing.value) {
    await store.refresh()
    // Kick off background polling so the table tracks external state
    // changes (someone stopping the instance via AWS console) and so
    // Start & apply feedback doesn't depend on the user mashing Refresh.
    store.startPolling()
  }
})

onBeforeUnmount(() => {
  // Covers `wails dev` hot reloads and the closing window lifecycle;
  // prevents the poller from leaking onto subsequent mounts.
  store.stopPolling()
})
</script>

<template>
  <div class="app">
    <header class="app-header">
      <div class="title">
        <span class="brand">ec2hosts</span>
        <span v-if="store.configInfo?.path" class="subtle" :title="store.configInfo.path">
          · {{ store.configInfo.path }}
        </span>
      </div>
      <div class="actions">
        <a @click="store.openCredsDialog">AWS credentials</a>
        <span class="sep">·</span>
        <a @click="store.openConfigInEditor">Edit config</a>
        <span class="sep">·</span>
        <a @click="store.openConfigFolder">Open folder</a>
      </div>
    </header>

    <AWSCredentialsDialog />

    <main v-if="configMissing" class="config-missing">
      <h2>config.yaml not found</h2>
      <p>
        ec2hosts looks for <code>config.yaml</code> next to the binary or at
        <code>%APPDATA%\ec2hosts\config.yaml</code>.
      </p>
      <p v-if="store.configInfo?.error" class="error">
        {{ store.configInfo.error }}
      </p>
      <button class="primary" @click="store.openConfigFolder">
        Open config folder
      </button>
    </main>

    <main v-else class="main">
      <EC2StatusBar :items="store.ec2" />
      <ActionButtons />
      <HostsTable :items="store.hosts" />
      <LogPanel />
    </main>
  </div>
</template>

<style scoped>
.app {
  display: flex;
  flex-direction: column;
  height: 100%;
}
.app-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 0.75rem 1rem;
  border-bottom: 1px solid var(--border);
  background: var(--panel);
}
.brand { font-weight: 600; }
.subtle { color: var(--muted); font-size: 0.85em; margin-left: 0.5rem; }
.actions { display: flex; gap: 0.5rem; align-items: center; font-size: 0.9em; }
.sep { color: var(--muted); }

.main {
  flex: 1;
  display: flex;
  flex-direction: column;
  gap: 1rem;
  padding: 1rem;
  overflow: hidden;
}

.config-missing {
  flex: 1;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  text-align: center;
  padding: 2rem;
  gap: 1rem;
}
.config-missing .error { color: var(--danger); font-family: monospace; }
code {
  background: var(--panel);
  padding: 0.1em 0.4em;
  border-radius: 4px;
  font-family: Consolas, Menlo, monospace;
}
</style>
