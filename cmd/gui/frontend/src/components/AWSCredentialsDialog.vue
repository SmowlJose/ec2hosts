<script setup>
import { ref, computed, watch, nextTick } from 'vue'
import { useAppStore } from '../stores/app'
import {
  AWSCredsStatus,
  AWSCredsTest,
  AWSCredsTestSaved,
  AWSCredsSave,
  OpenAWSFolder,
} from '../../wailsjs/go/main/App'

/*
 * AWSCredentialsDialog — rewritten against the "operations console"
 * palette so it doesn't look like a different app. Two-step Test →
 * Save flow is preserved, but the form now lives inside a right-hand
 * drawer that slides in over a dimmed backdrop, which reads closer to
 * a developer-tool inspector panel than a modal. Close on ESC, close
 * on backdrop click, everything keyboard-navigable.
 */

const store = useAppStore()

// Form state
const profile = ref('default')
const accessKeyId = ref('')
const secretAccessKey = ref('')
const sessionToken = ref('')
const region = ref('')
const showAdvanced = ref(false)

// Async state
const status = ref(null)
const loading = ref(false)
const testing = ref(false)
const saving = ref(false)
const identity = ref(null)
const savedOk = ref(false)
const testError = ref('')
const saveError = ref('')

const firstInput = ref(null)

async function refreshStatus() {
  loading.value = true
  try {
    status.value = await AWSCredsStatus()
    profile.value = status.value.activeProfile || 'default'
    region.value = status.value.region || ''
  } catch (e) {
    status.value = null
    testError.value = String(e)
  } finally {
    loading.value = false
  }
}

watch(
  () => store.showCredsDialog,
  async (open) => {
    if (!open) return
    accessKeyId.value = ''
    secretAccessKey.value = ''
    sessionToken.value = ''
    showAdvanced.value = false
    identity.value = null
    savedOk.value = false
    testError.value = ''
    saveError.value = ''
    await refreshStatus()
    await nextTick()
    firstInput.value?.focus()
  },
)

// Wire ESC-to-close at the window level so it works even when focus
// is deep inside a textarea. The listener is attached unconditionally
// and cheap; we just gate the action by dialog visibility.
function onKeydown(e) {
  if (e.key === 'Escape' && store.showCredsDialog) close()
}
if (typeof window !== 'undefined') {
  window.addEventListener('keydown', onKeydown)
}

const canTest = computed(
  () =>
    !testing.value &&
    !saving.value &&
    accessKeyId.value.trim() &&
    secretAccessKey.value.trim() &&
    profile.value.trim(),
)
const canSave = computed(() => canTest.value && identity.value)

async function test() {
  testError.value = ''
  saveError.value = ''
  identity.value = null
  testing.value = true
  try {
    identity.value = await AWSCredsTest({
      profile: profile.value.trim(),
      accessKeyId: accessKeyId.value.trim(),
      secretAccessKey: secretAccessKey.value.trim(),
      sessionToken: sessionToken.value.trim(),
      region: region.value.trim(),
    })
  } catch (e) {
    testError.value = humanize(e)
  } finally {
    testing.value = false
  }
}

async function save() {
  saveError.value = ''
  saving.value = true
  try {
    await AWSCredsSave({
      profile: profile.value.trim(),
      accessKeyId: accessKeyId.value.trim(),
      secretAccessKey: secretAccessKey.value.trim(),
      sessionToken: sessionToken.value.trim(),
      region: region.value.trim(),
    })
    savedOk.value = true
    await refreshStatus()
    await store.loadConfigInfo()
    store.refresh().catch(() => {})
  } catch (e) {
    saveError.value = humanize(e)
  } finally {
    saving.value = false
  }
}

async function testExisting() {
  testError.value = ''
  identity.value = null
  testing.value = true
  try {
    identity.value = await AWSCredsTestSaved()
  } catch (e) {
    testError.value = humanize(e)
  } finally {
    testing.value = false
  }
}

function close() { store.showCredsDialog = false }

function humanize(e) {
  const raw = String(e?.message || e)
  const marker = 'api error '
  const i = raw.indexOf(marker)
  if (i >= 0) return raw.slice(i + marker.length)
  return raw
}

// Paste-a-block heuristic (kept from the original): pasting the
// multi-line AWS-console credential block splits into the right fields.
function onPasteBundle(e) {
  const text = (e.clipboardData || window.clipboardData).getData('text') || ''
  if (!text.includes('\n')) return
  const lines = text.split(/\r?\n/).map((l) => l.trim()).filter(Boolean)
  if (lines.length < 2) return
  const akIdx = lines.findIndex((l) => /^(AKIA|ASIA)[A-Z0-9]{12,30}$/.test(l))
  if (akIdx < 0) return
  e.preventDefault()
  accessKeyId.value = lines[akIdx]
  const rest = lines.filter((_, i) => i !== akIdx)
  if (rest[0]) secretAccessKey.value = rest[0]
  const tokenLine = rest.find((l) => l.length > 100)
  if (tokenLine && tokenLine !== rest[0]) {
    sessionToken.value = tokenLine
    showAdvanced.value = true
  }
}
</script>

<template>
  <Transition name="drawer">
    <div v-if="store.showCredsDialog" class="shell" @click.self="close">
      <aside class="drawer" role="dialog" aria-labelledby="credsTitle">
        <!-- Head ————————————————————————————————————————————— -->
        <header class="head">
          <div>
            <p class="eyebrow label">signed session</p>
            <h2 id="credsTitle" class="display">
              AWS <em>credentials</em>
            </h2>
          </div>
          <button class="x" aria-label="Close" @click="close">✕</button>
        </header>

        <!-- Intro ——————————————————————————————————————————— -->
        <p class="intro">
          Writes to
          <code class="mono">{{ status?.credentialsPath || '%USERPROFILE%\\.aws\\credentials' }}</code>
          — the same file the AWS CLI reads. No extra tooling required.
        </p>

        <!-- Currently configured ——————————————————————————— -->
        <section v-if="status?.profileExists" class="current">
          <header class="current-head">
            <span class="label">current session</span>
            <span class="ok-dot" aria-hidden="true" />
          </header>
          <dl class="kv">
            <div class="kv-row">
              <dt class="label">Profile</dt>
              <dd class="mono">{{ status.activeProfile }}</dd>
            </div>
            <div class="kv-row">
              <dt class="label">Access key</dt>
              <dd class="mono">
                {{ status.maskedAccessKeyId }}
                <span v-if="status.hasSessionToken" class="tag">session</span>
              </dd>
            </div>
            <div v-if="status.region" class="kv-row">
              <dt class="label">Region</dt>
              <dd class="mono">{{ status.region }}</dd>
            </div>
          </dl>
          <div class="current-actions">
            <button :disabled="testing" @click="testExisting">
              {{ testing ? 'verifying…' : 'verify identity' }}
            </button>
            <a @click="OpenAWSFolder()">
              <span class="chev">›</span> open ~/.aws
            </a>
          </div>
          <div v-if="identity && !testError" class="ok">
            <span class="glyph">✓</span>
            <div>
              <div class="ok-title">authenticated</div>
              <div class="ok-body">
                <code class="mono">{{ identity.arn }}</code>
                <span class="sep">·</span>
                <span>account {{ identity.account }}</span>
              </div>
            </div>
          </div>
          <div v-if="testError" class="err">
            <span class="glyph">!</span>
            <div class="err-body">{{ testError }}</div>
          </div>
        </section>

        <!-- Form ——————————————————————————————————————————— -->
        <details :open="!status?.profileExists" class="form-wrap">
          <summary>
            <span class="chev">›</span>
            <span class="label">
              {{ status?.profileExists ? 'replace credentials' : 'set up credentials' }}
            </span>
          </summary>

          <div class="form">
            <label>
              <span class="field-label">Profile name</span>
              <input
                v-model="profile"
                type="text"
                autocomplete="off"
                spellcheck="false"
              />
              <span class="hint">
                Must match <code>aws.profile</code> in your config.yaml
                (or stay <code>default</code>).
              </span>
            </label>

            <label>
              <span class="field-label">Access key ID</span>
              <input
                ref="firstInput"
                v-model="accessKeyId"
                type="text"
                autocomplete="off"
                spellcheck="false"
                placeholder="AKIA…"
                @paste="onPasteBundle"
              />
            </label>

            <label>
              <span class="field-label">Secret access key</span>
              <input
                v-model="secretAccessKey"
                type="password"
                autocomplete="off"
                spellcheck="false"
                placeholder="••••••••••••••••••••"
              />
              <span class="hint">
                Stored plain-text in
                <code>~/.aws/credentials</code>, readable by your user only.
              </span>
            </label>

            <label>
              <span class="field-label">Region</span>
              <input
                v-model="region"
                type="text"
                autocomplete="off"
                spellcheck="false"
                placeholder="eu-west-1"
              />
              <span class="hint">Written to <code>~/.aws/config</code>.</span>
            </label>

            <div class="advanced-toggle">
              <a @click="showAdvanced = !showAdvanced">
                <span class="chev" :class="{ open: showAdvanced }">›</span>
                advanced — session token
              </a>
            </div>
            <label v-if="showAdvanced">
              <span class="field-label">Session token</span>
              <textarea
                v-model="sessionToken"
                rows="3"
                autocomplete="off"
                spellcheck="false"
                placeholder="only for temporary STS / SSO credentials"
              />
            </label>

            <div class="cta">
              <button
                class="primary"
                :disabled="!canTest"
                @click="test"
              >
                <span class="glyph">▶</span>
                <span>{{ testing ? 'testing…' : 'test' }}</span>
                <span class="kbd"><span>step 1</span></span>
              </button>
              <button
                class="primary"
                :disabled="!canSave"
                @click="save"
              >
                <span class="glyph">↓</span>
                <span>{{ saving ? 'saving…' : 'save' }}</span>
                <span class="kbd"><span>step 2</span></span>
              </button>
              <button @click="close">cancel</button>
            </div>

            <div v-if="identity && !testError && !savedOk" class="ok">
              <span class="glyph">✓</span>
              <div>
                <div class="ok-title">credentials valid</div>
                <div class="ok-body">
                  <code class="mono">{{ identity.arn }}</code>
                  <span class="sep">·</span>
                  <span>account {{ identity.account }}</span>
                </div>
                <div class="ok-hint">click <b>save</b> to write to disk.</div>
              </div>
            </div>
            <div v-if="savedOk" class="ok">
              <span class="glyph">✓</span>
              <div>
                <div class="ok-title">saved</div>
                <div class="ok-body">
                  close this drawer and hit <b>Start &amp; apply</b>.
                </div>
              </div>
            </div>
            <div v-if="testError && !identity" class="err">
              <span class="glyph">!</span>
              <div class="err-body">{{ testError }}</div>
            </div>
            <div v-if="saveError" class="err">
              <span class="glyph">!</span>
              <div class="err-body">{{ saveError }}</div>
            </div>
          </div>
        </details>
      </aside>
    </div>
  </Transition>
</template>

<style scoped>
.shell {
  position: fixed;
  inset: 0;
  z-index: 1000;
  display: flex;
  justify-content: flex-end;
  background: rgba(8, 7, 6, 0.55);
  backdrop-filter: blur(2px);
}

.drawer {
  position: relative;
  width: min(520px, 100vw);
  height: 100%;
  background: var(--surface-1);
  border-left: 1px solid var(--line-strong);
  overflow-y: auto;
  padding: 1.5rem 1.75rem 2rem;
  box-shadow: -20px 0 40px rgba(0, 0, 0, 0.35);
  display: flex;
  flex-direction: column;
  gap: 1.2rem;
}
/* A single decorative hairline down the left edge of the drawer, in
   amber. Signals "this is not main content, it's an inspector" without
   a huge header bar. */
.drawer::before {
  content: '';
  position: absolute;
  left: 0; top: 0; bottom: 0;
  width: 2px;
  background: var(--amber);
  opacity: 0.8;
}

/* ——— Head ————————————————————————————————————————————————— */

.head {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  gap: 1rem;
}
.eyebrow { margin: 0 0 0.3rem; color: var(--amber); }
h2 {
  margin: 0;
  font-family: var(--font-display);
  font-weight: 500;
  font-size: 28px;
  line-height: 1.05;
  letter-spacing: -0.02em;
  font-variation-settings: 'opsz' 144, 'SOFT' 40;
}
h2 em {
  font-style: italic;
  color: var(--amber);
  font-variation-settings: 'opsz' 144, 'SOFT' 100;
}
button.x {
  background: transparent;
  border: 1px solid var(--line);
  padding: 0;
  width: 28px;
  height: 28px;
  font-size: 11px;
  color: var(--ink-3);
  line-height: 1;
  text-transform: none;
  letter-spacing: 0;
}
button.x:hover {
  background: var(--surface-2);
  color: var(--ink);
  border-color: var(--line-strong);
}

/* ——— Intro ——————————————————————————————————————————————— */

.intro {
  margin: 0;
  color: var(--ink-2);
  font-size: 12.5px;
  line-height: 1.55;
}
.intro code { color: var(--ink); word-break: break-all; }

/* ——— Current session panel ————————————————————————————— */

.current {
  border: 1px solid var(--line);
  border-radius: var(--radius);
  background: var(--surface-2);
  padding: 0.9rem 1rem 0.95rem;
  display: flex;
  flex-direction: column;
  gap: 0.7rem;
}
.current-head {
  display: flex;
  justify-content: space-between;
  align-items: center;
}
.ok-dot {
  width: 7px;
  height: 7px;
  border-radius: 50%;
  background: var(--lime);
  box-shadow: 0 0 8px var(--lime-glow);
}
.kv {
  margin: 0;
  display: grid;
  grid-template-columns: auto 1fr;
  gap: 0.25rem 1rem;
}
.kv-row {
  display: contents;
}
.kv dt { margin: 0; }
.kv dd {
  margin: 0;
  color: var(--ink);
  font-size: 12.5px;
  display: inline-flex;
  align-items: center;
  gap: 0.5rem;
}
.tag {
  font-family: var(--font-mono);
  font-size: 9.5px;
  letter-spacing: 0.1em;
  text-transform: uppercase;
  padding: 0.08em 0.45em;
  background: var(--amber-wash);
  color: var(--amber);
  border: 1px solid rgba(232, 162, 59, 0.3);
  border-radius: 2px;
}
.current-actions {
  display: flex;
  align-items: center;
  gap: 1rem;
  font-size: 11px;
}

/* ——— Form ————————————————————————————————————————————————— */

.form-wrap { border-top: 1px solid var(--line); padding-top: 1rem; }
.form-wrap summary {
  cursor: pointer;
  list-style: none;
  display: flex;
  align-items: center;
  gap: 0.4rem;
  padding: 0.2rem 0;
}
.form-wrap summary::-webkit-details-marker { display: none; }
.form-wrap summary .chev {
  transition: transform 0.2s var(--ease);
  color: var(--ink-3);
  font-family: var(--font-mono);
}
.form-wrap[open] summary .chev { transform: rotate(90deg); }

.form {
  display: flex;
  flex-direction: column;
  gap: 0.9rem;
  margin-top: 0.8rem;
}
.form label {
  display: flex;
  flex-direction: column;
  gap: 0.3rem;
}
.field-label {
  font-family: var(--font-mono);
  font-size: 10.5px;
  text-transform: uppercase;
  letter-spacing: 0.14em;
  color: var(--ink-3);
}
.hint {
  color: var(--ink-3);
  font-size: 11px;
  line-height: 1.45;
}
.hint code {
  color: var(--ink-2);
  background: var(--surface-2);
  padding: 0.05em 0.35em;
  border-radius: 2px;
}

.advanced-toggle { font-size: 11px; }
.advanced-toggle a {
  color: var(--ink-3);
  text-transform: uppercase;
  letter-spacing: 0.12em;
  display: inline-flex;
  align-items: center;
  gap: 0.35rem;
  font-size: 10.5px;
  border-bottom: none;
}
.advanced-toggle a:hover { color: var(--amber); }
.advanced-toggle .chev {
  font-family: var(--font-mono);
  transition: transform 0.2s var(--ease);
}
.advanced-toggle .chev.open { transform: rotate(90deg); }

.cta {
  display: flex;
  flex-wrap: wrap;
  gap: 0.5rem;
  margin-top: 0.4rem;
}
.cta button .glyph {
  font-size: 9px;
  color: inherit;
}
.cta button .kbd {
  font-size: 9.5px;
  letter-spacing: 0.08em;
  text-transform: uppercase;
}

/* ——— Banners (ok / err) ——————————————————————————————————— */

.ok, .err {
  display: flex;
  align-items: flex-start;
  gap: 0.65rem;
  padding: 0.7rem 0.85rem;
  border-radius: var(--radius);
  font-size: 12px;
  line-height: 1.5;
}
.ok {
  color: var(--lime-glow);
  background: var(--lime-wash);
  border: 1px solid rgba(185, 220, 91, 0.25);
}
.err {
  color: var(--ember-hot);
  background: var(--ember-wash);
  border: 1px solid rgba(232, 100, 69, 0.3);
  font-family: var(--font-mono);
  word-break: break-word;
}
.ok .glyph, .err .glyph {
  flex-shrink: 0;
  width: 18px;
  height: 18px;
  border-radius: 50%;
  border: 1px solid currentColor;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  font-size: 10px;
  font-family: var(--font-mono);
  font-weight: 700;
  translate: 0 1px;
}
.ok-title {
  font-family: var(--font-mono);
  font-size: 10.5px;
  text-transform: uppercase;
  letter-spacing: 0.14em;
  color: var(--lime);
  margin-bottom: 0.15rem;
}
.ok-body { color: var(--ink-2); font-size: 11.5px; }
.ok-body code { color: var(--ink); }
.ok-body .sep { color: var(--ink-4); margin: 0 0.35rem; }
.ok-hint { color: var(--ink-3); font-size: 11px; margin-top: 0.25rem; }

.err-body { color: var(--ember-hot); font-size: 11.5px; }

.chev { font-family: var(--font-mono); color: var(--ink-3); }

code {
  font-family: var(--font-mono);
  color: var(--ink);
  font-size: 11.5px;
}

/* ——— Transition ——————————————————————————————————————————— */

.drawer-enter-active, .drawer-leave-active {
  transition: background 0.22s var(--ease);
}
.drawer-enter-active .drawer, .drawer-leave-active .drawer {
  transition: transform 0.28s var(--ease-out), opacity 0.28s var(--ease-out);
}
.drawer-enter-from { background: transparent; }
.drawer-enter-from .drawer { transform: translateX(40px); opacity: 0; }
.drawer-leave-to { background: transparent; }
.drawer-leave-to .drawer { transform: translateX(40px); opacity: 0; }
</style>
