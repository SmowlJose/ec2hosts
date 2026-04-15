# GUI — smoke tests pre-release

No automated tests. Before publishing a new release (cutting a `v*` tag),
walk through the following checklist on a clean Windows environment.
A throwaway VM or a second user account is ideal so `%APPDATA%` is
untouched.

## Environment

- **OS:** Windows 11 (minimum) or Windows 10 build 19041+.
- **User:** local admin (UAC = one click, no password).
- **AWS:** `~/.aws/credentials` with an IAM user allowed to
  `ec2:StartInstances`, `ec2:StopInstances`, `ec2:DescribeInstances`
  on the test instance.
- **Clean slate:** before step 1, verify there is no prior
  `%LOCALAPPDATA%\Programs\ec2hosts\` and no `%APPDATA%\ec2hosts\`.

## Checklist

- [ ] **1. Install.** Run `ec2hosts-gui-amd64-installer.exe`.
  - Expect: UAC prompt (installer itself elevates).
  - After finish: desktop shortcut **ec2hosts** visible;
    Start menu entry present; entry in "Add or remove programs".

- [ ] **2. WebView2 check.** On a VM without WebView2 pre-installed,
  step 1 should trigger the Evergreen bootstrapper automatically. On
  current Windows 11 this is typically a no-op.

- [ ] **3. First run without config.**
  - Open the app. Expect: "config.yaml not found" view with a button
    **Open config folder**.
  - Click the button; Explorer opens `%APPDATA%\ec2hosts\`. Confirm
    `config.yaml` is present (seeded from `config.yaml.example` by the
    installer). If it's missing, the seeding step failed — investigate.

- [ ] **4. First run without AWS credentials.** Rename
  `~/.aws/credentials` temporarily, relaunch.
  - Expect: action buttons visible, but **Refresh** / **Start & apply**
    surface a clear error in the log panel mentioning credentials. No
    crash. Restore credentials before continuing.

- [ ] **5. Refresh.** With valid config + creds, click **Refresh**.
  - Expect: the EC2 badge shows the current state (`stopped` if the
    instance is off), the hosts table lists every host from
    `config.yaml`, log says `status refreshed`.

- [ ] **6. Start & apply.** Click the green button.
  - Expect (log panel, in order): `starting i-xxx…`,
    `resolving public IP for i-xxx…`, `→ i-xxx = <ip>`,
    `writing hosts file…`, `requesting UAC elevation…`, UAC prompt,
    after accepting: `hosts file updated (N entries)`.
  - Verify in an elevated PowerShell:
    `Get-Content C:\Windows\System32\drivers\etc\hosts` — the
    `# BEGIN ec2hosts` block contains every host with the expected IP.

- [ ] **7. Refresh after Up.** Badge should flip to `running`; IPs in
  the table match what just got written to `hosts`.

- [ ] **8. Stop.** Click the red button.
  - Expect: UAC is NOT requested (stop does not touch hosts).
  - Log: `stopping i-xxx…`, then `done`. After a short wait a
    Refresh shows `stopping` then `stopped`.

- [ ] **9. Edit config.** Click **Edit config** in the header.
  - Expect: `config.yaml` opens in the default editor. Close without
    changes.

- [ ] **10. Uninstall.** Add or remove programs → ec2hosts → uninstall.
  - Expect: UAC prompt, progress dialog, then entry vanishes.
  - Verify: `%LOCALAPPDATA%\Programs\ec2hosts\` is gone; desktop +
    Start shortcuts gone; `%APPDATA%\ec2hosts\config.yaml` is
    **preserved** (intentional — it holds user configuration).

## When something fails

- The NSIS installer writes a log to `%TEMP%\ec2hosts-gui-install.log`
  if passed `/L=`. For ad-hoc debugging: `installer.exe /L=c:\log.txt`.
- Wails app crashes dump to `%APPDATA%\ec2hosts-gui\logs\`. The most
  recent file usually has the relevant stack.
- UAC prompts not appearing: verify the CLI binary was installed
  alongside the GUI (`%LOCALAPPDATA%\Programs\ec2hosts\ec2hosts.exe`).
  The GUI re-execs it for the privileged write.
