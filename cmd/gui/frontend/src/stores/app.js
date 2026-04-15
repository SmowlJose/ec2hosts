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

// Background polling cadence (ms). Fast while any instance is in a
// transitional state (pending, stopping, shutting-down...) so the user
// sees Start/Stop take effect within a couple of seconds; slow when
// everything is steady so we're not hammering DescribeInstances on
// AWS's dime. Values are generous — EC2 state transitions are 5–30 s.
const POLL_FAST_MS = 3000
const POLL_SLOW_MS = 15000

// Central store: one source of truth for what the UI renders.
export const useAppStore = defineStore('app', {
  state: () => ({
    configInfo: null,       // ConfigInfoDTO from the Go side
    ec2: [],                // StatusDTO[]
    hosts: [],              // HostDTO[]
    log: [],                // { level: 'info' | 'error', msg, ts }
    busy: false,            // true while Up/Down/refresh is running
    lastError: null,        // string or null
    // AWS credentials setup dialog. Toggled from the header link and
    // auto-opened when Status/Up/Down reports a credentials-shaped
    // error so the user lands in the right place instead of staring
    // at a cryptic stack trace.
    showCredsDialog: false,
    // Background polling handle. We keep it on the store (not as a
    // module-level var) so hot-reload during `wails dev` doesn't leave
    // zombie intervals that keep hitting AWS forever.
    _pollTimer: null,
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

    // Explicit user-initiated refresh — sets busy, logs to the panel,
    // and disables buttons. Use this for the Refresh button and after
    // a manual Up/Down.
    async refresh() {
      if (this.busy) return
      this.busy = true
      try {
        await this._pullStatus()
        this.pushLog('info', 'status refreshed')
      } finally {
        this.busy = false
      }
    },

    // Low-priority background poll. Never flips `busy` (so it doesn't
    // disable buttons) and never logs on success — only the UI table
    // updates. Errors are swallowed here and only surfaced on the
    // next user-initiated action, because a transient DescribeInstances
    // failure in the background shouldn't interrupt the user.
    async pollStatus() {
      if (this.busy) return              // defer to the in-flight action
      if (!this.configInfo?.found) return
      try {
        await this._pullStatus({ silent: true })
      } catch (_) {
        // intentionally ignored — see comment above
      }
    },

    // Shared request path for both manual and polled refresh. Returns
    // nothing, writes to this.ec2/this.hosts directly, and re-throws
    // on failure so callers can decide whether to log it.
    async _pullStatus({ silent = false } = {}) {
      try {
        const [ec2, hosts] = await Promise.all([Status(), ReadHosts()])
        this.ec2 = ec2 || []
        this.hosts = hosts || []
        this.lastError = null
      } catch (e) {
        if (!silent) {
          this.lastError = String(e)
          this.pushLog('error', String(e))
          if (this.isCredentialsError(e?.message || e)) this.showCredsDialog = true
        }
        throw e
      }
    },

    // Start the background poller. Rate adapts: fast (3 s) while any
    // instance is transitioning between states, slow (15 s) once
    // everything is stable. Re-entry-safe — calling start twice is a
    // no-op.
    startPolling() {
      if (this._pollTimer) return
      const tick = async () => {
        await this.pollStatus()
        // Recompute interval each tick so a transition triggered
        // externally (e.g. the user stopping the instance via AWS
        // console) ramps us up within one slow cycle.
        const anyTransitional = this.ec2.some(
          (s) =>
            s.state === 'pending' ||
            s.state === 'stopping' ||
            s.state === 'shutting-down' ||
            s.state === 'rebooting',
        )
        const delay = anyTransitional ? POLL_FAST_MS : POLL_SLOW_MS
        this._pollTimer = setTimeout(tick, delay)
      }
      // Kick off the first tick after POLL_SLOW_MS so we don't double
      // up with the initial refresh in onMounted.
      this._pollTimer = setTimeout(tick, POLL_SLOW_MS)
    },

    stopPolling() {
      if (this._pollTimer) {
        clearTimeout(this._pollTimer)
        this._pollTimer = null
      }
    },

    async up() {
      if (this.busy) return
      this.busy = true
      this.lastError = null
      try {
        await Up()
        // After a successful Up, refresh state so the table shows fresh IPs.
        await this._pullStatus()
      } catch (e) {
        this.lastError = String(e)
        this.pushLog('error', String(e))
        if (this.isCredentialsError(e?.message || e)) this.showCredsDialog = true
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
        await this._pullStatus()
      } catch (e) {
        this.lastError = String(e)
        this.pushLog('error', String(e))
        if (this.isCredentialsError(e?.message || e)) this.showCredsDialog = true
      } finally {
        this.busy = false
      }
    },

    openConfigInEditor() { OpenConfigInEditor().catch((e) => this.pushLog('error', String(e))) },
    openConfigFolder()   { OpenConfigFolder().catch((e) => this.pushLog('error', String(e))) },

    openCredsDialog()  { this.showCredsDialog = true  },
    closeCredsDialog() { this.showCredsDialog = false },

    // Heuristic: AWS SDK errors for missing/invalid credentials come
    // back with fairly stable substrings. If refresh() or up() hits
    // one, we surface the credentials dialog so the user can fix the
    // root cause in two clicks instead of parsing a 300-char SDK trace.
    isCredentialsError(msg) {
      const s = String(msg || '').toLowerCase()
      return (
        s.includes('no ec2 imds') ||
        s.includes('failed to retrieve credentials') ||
        s.includes('nocredentialproviders') ||
        s.includes('invalidclienttokenid') ||
        s.includes('signaturedoesnotmatch') ||
        s.includes('expiredtoken') ||
        s.includes('unablelocatecredentials')
      )
    },

    pushLog(level, msg) {
      this.log.push({ level, msg, ts: new Date() })
      // Cap the log to the last 200 lines to avoid unbounded growth.
      if (this.log.length > 200) this.log.splice(0, this.log.length - 200)
    },

    clearLog() { this.log = [] },
  },
})
