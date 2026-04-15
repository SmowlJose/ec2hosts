import { defineStore } from 'pinia'

// Wails auto-generates these bindings under frontend/wailsjs/ during
// `wails dev` / `wails build`. The files are only present at build time,
// so the import is resolved by the dev server / bundler — not by hand.
import {
  ConfigInfo,
  Status,
  ReadHosts,
  Up,
  Down,
  OpenConfigInEditor,
  OpenConfigFolder,
} from '../../wailsjs/go/main/App'
import { EventsOn } from '../../wailsjs/runtime/runtime'

// Central store: one source of truth for what the UI renders.
export const useAppStore = defineStore('app', {
  state: () => ({
    configInfo: null,       // ConfigInfoDTO from the Go side
    ec2: [],                // StatusDTO[]
    hosts: [],              // HostDTO[]
    log: [],                // { level: 'info' | 'error', msg, ts }
    busy: false,            // true while Up/Down/refresh is running
    lastError: null,        // string or null
  }),

  actions: {
    // One-time subscription to backend progress events. Called from
    // App.vue on mount. Safe to call multiple times (Wails dedupes).
    subscribeEvents() {
      EventsOn('progress', (msg) => this.pushLog('info', msg))
    },

    async loadConfigInfo() {
      try {
        this.configInfo = await ConfigInfo()
        if (this.configInfo.error) {
          this.lastError = this.configInfo.error
        }
      } catch (e) {
        this.lastError = String(e)
      }
    },

    async refresh() {
      if (this.busy) return
      this.busy = true
      this.lastError = null
      try {
        const [ec2, hosts] = await Promise.all([Status(), ReadHosts()])
        this.ec2 = ec2 || []
        this.hosts = hosts || []
        this.pushLog('info', 'status refreshed')
      } catch (e) {
        this.lastError = String(e)
        this.pushLog('error', String(e))
      } finally {
        this.busy = false
      }
    },

    async up() {
      if (this.busy) return
      this.busy = true
      this.lastError = null
      try {
        await Up()
        // After a successful Up, refresh state so the table shows fresh IPs.
        const [ec2, hosts] = await Promise.all([Status(), ReadHosts()])
        this.ec2 = ec2 || []
        this.hosts = hosts || []
      } catch (e) {
        this.lastError = String(e)
        this.pushLog('error', String(e))
      } finally {
        this.busy = false
      }
    },

    async down() {
      if (this.busy) return
      this.busy = true
      this.lastError = null
      try {
        await Down()
        const ec2 = await Status()
        this.ec2 = ec2 || []
      } catch (e) {
        this.lastError = String(e)
        this.pushLog('error', String(e))
      } finally {
        this.busy = false
      }
    },

    openConfigInEditor() { OpenConfigInEditor().catch((e) => this.pushLog('error', String(e))) },
    openConfigFolder()   { OpenConfigFolder().catch((e) => this.pushLog('error', String(e))) },

    pushLog(level, msg) {
      this.log.push({ level, msg, ts: new Date() })
      // Cap the log to the last 200 lines to avoid unbounded growth.
      if (this.log.length > 200) this.log.splice(0, this.log.length - 200)
    },

    clearLog() { this.log = [] },
  },
})
