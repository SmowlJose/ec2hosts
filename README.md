# ec2hosts

Starts your AWS EC2 instance(s) and points a configurable list of
hostnames to them — or to your own machine, or to any static IP — by
managing a single delimited block inside the system `hosts` file.

## Install

### From GitHub Releases (recommended)

Download the binary for your OS/arch from the
[Releases page](https://github.com/SmowlJose/ec2hosts/releases).

**Linux / macOS**

```bash
# pick the right asset for your platform
curl -L -o ec2hosts https://github.com/SmowlJose/ec2hosts/releases/latest/download/ec2hosts-linux-amd64
chmod +x ec2hosts
sudo mv ec2hosts /usr/local/bin/
```

**Windows — GUI (recommended)**

Download `ec2hosts-gui-amd64-installer.exe` from the releases page and run
it. The installer:

- Installs the GUI to `%LOCALAPPDATA%\Programs\ec2hosts\`.
- Creates a desktop shortcut and a Start menu entry.
- Bundles the CLI binary alongside the GUI (used internally for the
  privileged hosts-file write via UAC).
- Seeds `%APPDATA%\ec2hosts\config.yaml` from the example on first
  install (preserves the file on reinstall/upgrade).
- Requires WebView2 (bundled on Windows 11 and current Windows 10; the
  installer downloads it on older systems).

On first run, if no `config.yaml` exists yet, the app points you to the
config folder so you can drop one in. Builds the same config schema as
the CLI — see [Configuration](#configuration) below.

The installer is unsigned; on first download SmartScreen shows "Windows
protected your PC". Click **More info** → **Run anyway**. This is by
design for v1.

**Windows — CLI**

Download `ec2hosts-windows-amd64.exe` from the releases page, rename it
to `ec2hosts.exe`, and drop it somewhere in your `PATH`. Run from an
elevated PowerShell (or let the GUI installer place it next to the
GUI; the same binary is reused).

### Build from source

Requires Go 1.22+.

```bash
git clone https://github.com/SmowlJose/ec2hosts.git
cd ec2hosts
go build -o ec2hosts ./cmd/cli
```

Cross-compile the CLI:

```bash
GOOS=linux   GOARCH=amd64 go build -o dist/ec2hosts-linux-amd64   ./cmd/cli
GOOS=darwin  GOARCH=arm64 go build -o dist/ec2hosts-darwin-arm64  ./cmd/cli
GOOS=windows GOARCH=amd64 go build -o dist/ec2hosts-windows-amd64.exe ./cmd/cli
```

Build the Windows GUI (run on Windows, requires Go + Node + [Wails v2](https://wails.io)):

```powershell
go install github.com/wailsapp/wails/v2/cmd/wails@v2.9.2
cd cmd/gui
wails build -platform windows/amd64 -nsis
```

Output lands in `cmd/gui/build/bin/`.

## Quick start

1. Configure AWS credentials once, if you have not already:

   ```bash
   aws configure           # or: aws sso login --profile my-profile
   ```

2. Create a `config.yaml` next to the binary (or at
   `~/.config/ec2hosts/config.yaml`). Copy
   [`config.yaml.example`](./config.yaml.example) as a starting point.

3. Start everything:

   ```bash
   ec2hosts up
   ```

   Output looks like:

   ```
   starting i-0123456789abcdef0 (target=ec2)... running
   resolving public IP for i-0123456789abcdef0... 52.211.58.12
   hosts file not writable as user, escalating via sudo...
   [sudo] password for jose:
   /etc/hosts updated (17 entries)
   ```

## Configuration

A minimal `config.yaml`:

```yaml
aws:
  region: eu-west-1
  # profile: my-profile      # optional; falls back to AWS_PROFILE / default

# hosts_file: /etc/hosts     # optional — autodetected per OS
marker_tag: ec2hosts         # used in the BEGIN/END block markers

default_target: ec2          # target used by entries without an explicit one

targets:
  ec2:
    type: ec2
    instance_id: i-0123456789abcdef0
  local:
    type: static
    ip: 127.0.0.1
  # teammate:
  #   type: static
  #   ip: 10.0.0.42

hosts:
  - host: admin-local.example.com        # inherits default_target (ec2)
  - host: api-local.example.com
  - host: webapp-local.example.com
    target: local                        # this one points to your machine
  - host: live-local.example.com
    target: local
```

### `targets`

Named IP sources. Two types:

| Type     | Extra fields         | Resolves to                                |
| -------- | -------------------- | ------------------------------------------ |
| `ec2`    | `instance_id`        | the instance's current public IP (via AWS) |
| `static` | `ip`                 | the literal IP                             |

### `hosts`

A list of `{host, target}` pairs. If `target` is omitted, `default_target`
is used. This is how you decide **which hostnames go to EC2 and which
stay on your machine** (or elsewhere).

### Hosts file location

Autodetected:

- Linux / macOS → `/etc/hosts`
- Windows → `%SystemRoot%\System32\drivers\etc\hosts`

Override with `hosts_file:` if needed.

### Marker tag

Only the block between `# BEGIN <tag>` and `# END <tag>` is managed.
Anything outside those markers is left untouched. Change `marker_tag` if
you ever need to run two independent copies on the same machine.

## Commands

| Command                     | What it does                                                                  |
| --------------------------- | ----------------------------------------------------------------------------- |
| `ec2hosts up`               | Start every EC2 instance referenced by some host and rewrite the hosts file. |
| `ec2hosts apply`            | Rewrite the hosts file using cached IPs (live-resolves any missing target).  |
| `ec2hosts status`           | Show the current EC2 instance state and the managed block.                   |
| `ec2hosts down`             | Stop every EC2 instance referenced by the config.                            |
| `ec2hosts switch HOST TGT`  | Rewrite the config so `HOST` points to target `TGT`, then `apply`.           |
| `ec2hosts restore`          | Remove the managed block from the hosts file.                                |

Global flags:

- `--config PATH` — path to `config.yaml` (default: `./config.yaml`, then
  `$XDG_CONFIG_HOME/ec2hosts/config.yaml`).
- `--dry-run` — print what would change without touching the hosts file.
- `-h`, `--help` — show help.

## Features

- Per-host routing: every hostname picks a named **target** (EC2
  instance, local machine, teammate's IP, …).
- Single static binary. No Python, no `pip`, no virtualenv.
- Idempotent `# BEGIN ec2hosts` / `# END ec2hosts` block — user entries
  are never touched.
- Automatic timestamped backup of the hosts file before every write.
- Atomic write (temp file + `rename`), so `/etc/hosts` is never left
  half-written.
- Privilege separation: AWS calls run as your user; only the actual
  write to `/etc/hosts` auto-escalates via `sudo`.
- Uses the standard AWS SDK credential chain (`~/.aws/credentials`,
  `AWS_PROFILE`, SSO, IAM roles) — no keys in config files.
- IP cache in `~/.cache/ec2hosts/state.json` so `apply` can reuse the
  last known IP without calling AWS.
- `--dry-run` to preview the proposed hosts file.

### Typical workflows

```bash
# Morning — boot the dev instance and point every host at it
ec2hosts up

# I want webapp to hit my local dev server instead of EC2
ec2hosts switch webapp-local.example.com local
ec2hosts apply

# Quickly preview what ec2hosts would write
ec2hosts --dry-run apply

# End of day — stop the instance
ec2hosts down

# Revert /etc/hosts to its original state
ec2hosts restore
```

## How privileges work

**Do not run `ec2hosts` with `sudo`.**

`sudo` clears `$HOME` and `AWS_*` environment variables, so the SDK
cannot find your credentials and falls back to EC2 IMDS (which never
answers outside EC2). The error you'd see is:

```
get credentials: failed to refresh cached credentials,
no EC2 IMDS role found … context deadline exceeded
```

Instead, `ec2hosts` runs the AWS calls as your user and only re-execs
itself with elevated privileges for the single step that needs root —
writing the hosts file. The parent and the hidden `__write-hosts` child
talk over a short-lived JSON payload:

- **Linux / macOS:** re-exec via `sudo`, payload piped on stdin. `sudo`
  reads the password from `/dev/tty`, so the stdin pipe and the password
  prompt do not clash.
- **Windows (CLI and GUI):** re-exec via `ShellExecuteEx` with the
  `runas` verb, which triggers the standard UAC prompt. The payload
  travels through a temp file in `%TEMP%` (stdin cannot be piped into
  an elevated child under this API). Stale temp payloads older than an
  hour are cleaned up on every startup.

The elevation helper lives in [`internal/elevate`](./internal/elevate/)
and is shared between CLI and GUI — one code path for the privileged
write, regardless of OS or binary.

## AWS credentials

`ec2hosts` does not read credentials from its own config. It uses the
SDK default resolution order:

1. Environment variables (`AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY`,
   `AWS_SESSION_TOKEN`).
2. Shared credentials file (`~/.aws/credentials`).
3. Shared config file (`~/.aws/config`) — picks up SSO sessions.
4. Container / IMDS roles (only useful when running on AWS itself).

Set `aws.profile` in `config.yaml` or `AWS_PROFILE` in your shell to
pick a specific profile.

## Troubleshooting

**`no config found (tried: …) — pass --config PATH`**
Create `./config.yaml` or point `--config` at the right file.

**`get credentials: failed to refresh cached credentials … IMDS … context deadline exceeded`**
You ran with `sudo`, or your AWS credentials are not configured. Run
without `sudo` and make sure `aws sts get-caller-identity` works.

**`operation error EC2: StartInstances … UnauthorizedOperation`**
The IAM identity you are using cannot start the instance. You need
`ec2:StartInstances`, `ec2:StopInstances`, and `ec2:DescribeInstances`
on the target instance ID.

**The hosts file was not updated**
Run `ec2hosts --dry-run apply` to see exactly what would be written,
then `ec2hosts status` to see the block currently in the file.

**I edited `/etc/hosts` by hand and now things look weird**
`ec2hosts` only touches content between `# BEGIN <tag>` and
`# END <tag>`. Anything outside that block is safe. Use
`ec2hosts restore` to remove the managed block entirely, then
re-apply.

## Layout

```
.
├── cmd/
│   ├── cli/                      # ec2hosts CLI entry point
│   │   └── main.go
│   └── gui/                      # Windows-only GUI (Wails v2 + Vue 3)
│       ├── main.go               # Wails bootstrap (//go:build windows)
│       ├── app.go                # methods exposed to the frontend
│       ├── wails.json            # Wails project config
│       ├── build/windows/        # manifest, info.json, NSIS installer
│       ├── frontend/             # Vue 3 + Vite + Pinia
│       └── TESTING.md            # manual smoke checklist for releases
├── internal/
│   ├── awsec2/                   # aws-sdk-go-v2 wrapper
│   ├── config/                   # YAML schema + validation
│   ├── elevate/                  # sudo / UAC elevation helper
│   ├── hosts/                    # idempotent block editor + atomic writes
│   └── state/                    # IP cache
├── config.yaml.example
├── docs/superpowers/specs/       # design docs
└── .github/workflows/release.yml # builds CLI + GUI installer on tag v*
```

The CLI and the GUI share everything under `internal/`. There is a single
source of truth for AWS calls, the hosts-file editor, the config parser,
and the elevation step — the GUI is a thin Wails shell over those.
