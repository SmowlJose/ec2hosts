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
 * AWSCredentialsDialog — a Windows-friendly replacement for running
 * `aws configure`. Writes to %USERPROFILE%\.aws\credentials directly.
 *
 * UX goals (in order of priority):
 *
 *   1. Users who already have credentials see that state immediately
 *      ("Signed in as <arn>") without the dialog getting in the way.
 *   2. Users who don't have credentials see a pre-filled form with
 *      sensible defaults (profile name and region taken from
 *      config.yaml) — no typing required for those two fields.
 *   3. The user can Test before Save, so a typo doesn't silently
 *      produce a broken ~/.aws/credentials.
 *   4. Secrets are password-masked and not logged in the event stream.
 *
 * Rendering note: the dialog is its own overlay so it stacks above the
 * main layout even while Up/Down is running. We intentionally do NOT
 * disable the parent with store.busy — those workflows are unrelated.
 */

const store = useAppStore()

// Local form state, reset every time the dialog opens.
const profile = ref('default')
const accessKeyId = ref('')
const secretAccessKey = ref('')
const sessionToken = ref('')
const region = ref('')
const showAdvanced = ref(false)

// Async state: status from backend + the result of a Test/Save click.
const status = ref(null)         // AWSCredsStatusDTO or null while loading
const loading = ref(false)
const testing = ref(false)
const saving = ref(false)
const savedIdentity = ref(null)  // { account, arn, userId } after a successful test
const savedOk = ref(false)       // true once we've both tested and saved
const testError = ref('')
const saveError = ref('')

const firstInput = ref(null)

// Keep the two side effects of opening the dialog colocated so we
// don't get into a half-open state if one of them throws.
async function refreshStatus() {
  loading.value = true
  try {
    status.value = await AWSCredsStatus()
    // Pre-fill the form from whatever the backend knows.
    profile.value = status.value.activeProfile || 'default'
    region.value = status.value.region || ''
  } catch (e) {
    status.value = null
    testError.value = String(e)
  } finally {
    loading.value = false
  }
}

// Auto-focus the first editable field (access key ID) when the dialog
// becomes visible. nextTick lets Vue finish rendering first.
watch(
  () => store.showCredsDialog,
  async (open) => {
    if (!open) return
    // Reset transient form state on each open.
    accessKeyId.value = ''
    secretAccessKey.value = ''
    sessionToken.value = ''
    showAdvanced.value = false
    savedIdentity.value = null
    savedOk.value = false
    testError.value = ''
    saveError.value = ''
    await refreshStatus()
    await nextTick()
    firstInput.value?.focus()
  },
  { immediate: false },
)

const canTest = computed(
  () =>
    !testing.value &&
    !saving.value &&
    accessKeyId.value.trim() &&
    secretAccessKey.value.trim() &&
    profile.value.trim(),
)

const canSave = computed(() => canTest.value && savedIdentity.value)

async function test() {
  testError.value = ''
  saveError.value = ''
  savedIdentity.value = null
  testing.value = true
  try {
    savedIdentity.value = await AWSCredsTest({
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
    // Re-read status so the "Currently configured as …" panel updates
    // to reflect what we just wrote, and the config info banner in
    // the main view picks up the new region if it changed.
    await refreshStatus()
    await store.loadConfigInfo()
    // Auto-refresh EC2 status so the user sees the change take effect.
    store.refresh().catch(() => {})
  } catch (e) {
    saveError.value = humanize(e)
  } finally {
    saving.value = false
  }
}

async function testExisting() {
  testError.value = ''
  savedIdentity.value = null
  testing.value = true
  try {
    savedIdentity.value = await AWSCredsTestSaved()
  } catch (e) {
    testError.value = humanize(e)
  } finally {
    testing.value = false
  }
}

function close() {
  store.showCredsDialog = false
}

// A best-effort cleanup of AWS SDK error strings. The SDK leaks raw
// "operation error STS: GetCallerIdentity, https response error..."
// prose; we trim the most common noise so the surface area the user
// reads is the actionable part (InvalidClientTokenId, SignatureDoesNotMatch, etc.).
function humanize(e) {
  const raw = String(e?.message || e)
  const marker = 'api error '
  const i = raw.indexOf(marker)
  if (i >= 0) return raw.slice(i + marker.length)
  return raw
}

// Paste-a-block heuristic: if the user pastes a chunk that looks like
// the output of `aws sts assume-role` or the "access keys" block from
// the AWS console ("AKIA...\nwJalr...\nIQoJb...\n"), try to split it
// into the right fields. Keeps the user out of "which line goes where"
// territory.
function onPasteBundle(e) {
  const text = (e.clipboardData || window.clipboardData).getData('text') || ''
  if (!text.includes('\n')) return // not a multiline paste, let the native handler run
  const lines = text
    .split(/\r?\n/)
    .map((l) => l.trim())
    .filter(Boolean)
  if (lines.length < 2) return

  // Heuristic: exactly one line looks like an access key id (starts
  // with AKIA/ASIA and is 16–32 chars), treat the next non-empty line
  // as the secret, and any obvious token line (Base64-ish, >100 chars)
  // as session token.
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
  <div v-if="store.showCredsDialog" class="backdrop" @click.self="close">
    <div class="dialog" role="dialog" aria-labelledby="awsCredsTitle">
      <header class="head">
        <h2 id="awsCredsTitle">AWS credentials</h2>
        <button class="icon" aria-label="Close" @click="close">×</button>
      </header>

      <div class="body">
        <p class="intro">
          ec2hosts uses the standard AWS credentials file at
          <code>{{ status?.credentialsPath || '%USERPROFILE%\\.aws\\credentials' }}</code>.
          If you don't have the AWS CLI installed, you can create or update
          that file here — no extra tooling required.
        </p>

        <!-- Currently-configured state: show it at the top so users
             with working creds never have to scroll through the form. -->
        <section v-if="status?.profileExists" class="current">
          <div class="current-row">
            <span class="label">Profile</span>
            <code>{{ status.activeProfile }}</code>
          </div>
          <div class="current-row">
            <span class="label">Access key</span>
            <code>{{ status.maskedAccessKeyId }}</code>
            <span v-if="status.hasSessionToken" class="badge">session token</span>
          </div>
          <div v-if="status.region" class="current-row">
            <span class="label">Region</span>
            <code>{{ status.region }}</code>
          </div>
          <div class="current-actions">
            <button :disabled="testing" @click="testExisting">
              {{ testing ? 'Testing…' : 'Test current credentials' }}
            </button>
            <a class="link" @click="OpenAWSFolder()">Open ~/.aws folder</a>
          </div>
          <div v-if="savedIdentity && !testError" class="ok">
            ✓ Signed in as <code>{{ savedIdentity.arn }}</code>
            (account {{ savedIdentity.account }})
          </div>
          <div v-if="testError" class="err">{{ testError }}</div>
        </section>

        <!-- Form for (re)setting credentials. Collapsed header when
             creds already exist, so we don't push the happy path
             under a wall of inputs. -->
        <details :open="!status?.profileExists" class="form-wrap">
          <summary>
            {{ status?.profileExists ? 'Replace credentials' : 'Set up credentials' }}
          </summary>

          <div class="form">
            <label>
              <span class="lbl">Profile name</span>
              <input
                v-model="profile"
                type="text"
                autocomplete="off"
                spellcheck="false"
              />
              <span class="hint">
                Must match <code>aws.profile</code> in your config.yaml
                (leave as <code>default</code> if you don't use profiles).
              </span>
            </label>

            <label>
              <span class="lbl">Access key ID</span>
              <input
                ref="firstInput"
                v-model="accessKeyId"
                type="text"
                autocomplete="off"
                spellcheck="false"
                placeholder="AKIA..."
                @paste="onPasteBundle"
              />
            </label>

            <label>
              <span class="lbl">Secret access key</span>
              <input
                v-model="secretAccessKey"
                type="password"
                autocomplete="off"
                spellcheck="false"
              />
              <span class="hint">
                Stored in plain text at <code>{{ status?.credentialsPath || '~/.aws/credentials' }}</code>,
                readable only by your user.
              </span>
            </label>

            <label>
              <span class="lbl">Region</span>
              <input
                v-model="region"
                type="text"
                autocomplete="off"
                spellcheck="false"
                placeholder="eu-west-1"
              />
              <span class="hint">Written to ~/.aws/config for this profile.</span>
            </label>

            <div class="advanced-toggle">
              <a class="link" @click="showAdvanced = !showAdvanced">
                {{ showAdvanced ? '▼' : '▶' }} Advanced (session token)
              </a>
            </div>
            <label v-if="showAdvanced">
              <span class="lbl">Session token</span>
              <textarea
                v-model="sessionToken"
                rows="3"
                autocomplete="off"
                spellcheck="false"
              ></textarea>
              <span class="hint">
                Required for temporary credentials from STS / SSO.
                Leave empty for long-lived IAM user keys.
              </span>
            </label>

            <!-- Two-step flow: Test first (no disk write) so a typo
                 is caught before we overwrite ~/.aws/credentials. -->
            <div class="cta">
              <button class="primary" :disabled="!canTest" @click="test">
                {{ testing ? 'Testing…' : 'Test' }}
              </button>
              <button class="primary" :disabled="!canSave" @click="save">
                {{ saving ? 'Saving…' : 'Save' }}
              </button>
              <button @click="close">Cancel</button>
            </div>

            <div v-if="savedIdentity && !testError" class="ok">
              ✓ Credentials valid — signed in as
              <code>{{ savedIdentity.arn }}</code>
              (account {{ savedIdentity.account }}).
              Click <b>Save</b> to write them to disk.
            </div>
            <div v-if="savedOk" class="ok">
              ✓ Saved. You can close this dialog and try <b>Start &amp; apply</b>.
            </div>
            <div v-if="testError" class="err">{{ testError }}</div>
            <div v-if="saveError" class="err">{{ saveError }}</div>
          </div>
        </details>
      </div>
    </div>
  </div>
</template>

<style scoped>
.backdrop {
  position: fixed;
  inset: 0;
  background: rgba(15, 23, 42, 0.4);
  display: flex;
  align-items: flex-start;
  justify-content: center;
  z-index: 1000;
  padding: 2rem 1rem;
  overflow-y: auto;
}
.dialog {
  background: var(--bg);
  border: 1px solid var(--border);
  border-radius: 10px;
  width: 100%;
  max-width: 560px;
  box-shadow: 0 20px 40px rgba(15, 23, 42, 0.15);
}
.head {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 0.9rem 1.2rem;
  border-bottom: 1px solid var(--border);
}
.head h2 { margin: 0; font-size: 1.05rem; }
button.icon {
  background: transparent;
  border: none;
  font-size: 1.3rem;
  line-height: 1;
  padding: 0 0.4rem;
  color: var(--muted);
}
button.icon:hover { color: var(--fg); background: transparent; }

.body {
  padding: 1.1rem 1.2rem 1.3rem;
  display: flex;
  flex-direction: column;
  gap: 1rem;
}
.intro {
  margin: 0;
  color: var(--muted);
  font-size: 0.92rem;
  line-height: 1.4;
}
.intro code {
  font-size: 0.9em;
  word-break: break-all;
}

.current {
  border: 1px solid var(--border);
  border-radius: 8px;
  padding: 0.75rem 0.9rem;
  background: var(--panel);
  display: flex;
  flex-direction: column;
  gap: 0.4rem;
}
.current-row {
  display: flex;
  align-items: center;
  gap: 0.6rem;
  font-size: 0.92rem;
}
.current-row .label {
  color: var(--muted);
  min-width: 86px;
}
.current-actions {
  display: flex;
  gap: 0.9rem;
  align-items: center;
  margin-top: 0.3rem;
}
.badge {
  background: var(--warn);
  color: #fff;
  border-radius: 10px;
  font-size: 0.7rem;
  padding: 0.05rem 0.5rem;
  letter-spacing: 0.02em;
}

.form-wrap summary {
  cursor: pointer;
  padding: 0.35rem 0;
  font-weight: 600;
}
.form-wrap[open] summary {
  margin-bottom: 0.6rem;
}
.form {
  display: flex;
  flex-direction: column;
  gap: 0.8rem;
}
.form label {
  display: flex;
  flex-direction: column;
  gap: 0.25rem;
  font-size: 0.92rem;
}
.form .lbl {
  font-weight: 500;
}
.form .hint {
  color: var(--muted);
  font-size: 0.82rem;
  line-height: 1.35;
}
.form input,
.form textarea {
  font: inherit;
  font-family: Consolas, Menlo, monospace;
  padding: 0.45rem 0.6rem;
  border: 1px solid var(--border);
  border-radius: 6px;
  background: #fff;
  color: var(--fg);
}
.form input:focus,
.form textarea:focus {
  outline: none;
  border-color: var(--accent);
  box-shadow: 0 0 0 3px rgba(37, 99, 235, 0.18);
}
.form textarea { resize: vertical; min-height: 4.5em; }

.advanced-toggle { font-size: 0.88rem; }
.advanced-toggle .link { user-select: none; }

.cta {
  display: flex;
  gap: 0.5rem;
  flex-wrap: wrap;
  margin-top: 0.4rem;
}

.ok {
  color: var(--success);
  background: #f0fdf4;
  border: 1px solid #bbf7d0;
  border-radius: 6px;
  padding: 0.55rem 0.75rem;
  font-size: 0.9rem;
}
.err {
  color: var(--danger);
  background: #fef2f2;
  border: 1px solid #fecaca;
  border-radius: 6px;
  padding: 0.55rem 0.75rem;
  font-size: 0.9rem;
  font-family: Consolas, Menlo, monospace;
  word-break: break-word;
}
.link { cursor: pointer; color: var(--accent); font-size: 0.9rem; }
.link:hover { text-decoration: underline; }
code {
  background: var(--panel);
  padding: 0.08em 0.35em;
  border-radius: 4px;
  font-family: Consolas, Menlo, monospace;
  font-size: 0.9em;
}
</style>
