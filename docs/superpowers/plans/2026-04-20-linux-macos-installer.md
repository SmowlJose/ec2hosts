# Linux/macOS installer (`install.sh`)

Date: 2026-04-20

## Goal

One-liner install for Linux/macOS that mirrors what the Windows GUI
installer does: fetch the binary, put it on `PATH`, and seed a starter
config — without clobbering anything the user has already set up.

## Usage

```bash
curl -fsSL https://raw.githubusercontent.com/SmowlJose/ec2hosts/main/install.sh | bash
```

## Overrides

| env var              | effect                                  |
|----------------------|-----------------------------------------|
| `EC2HOSTS_VERSION`   | release tag instead of `latest`         |
| `EC2HOSTS_BIN_DIR`   | force install dir                       |
| `EC2HOSTS_NO_PATH`   | skip rc-file PATH wiring                |
| `EC2HOSTS_NO_CONFIG` | skip config seed                        |

## Behavior

1. **Download** `ec2hosts-<os>-<arch>` from GitHub Releases. Magic-byte
   sanity check (ELF / Mach-O). Temp dir removed on exit via a guarded
   `trap`.
2. **Install dir** (in order):
   - `$EC2HOSTS_BIN_DIR` if set,
   - else `/usr/local/bin` (direct write if writable, or passwordless
     `sudo -n`),
   - else `$HOME/.local/bin`.
3. **PATH wiring**: if install dir is not already on `PATH`, append an
   idempotent block guarded by the marker
   `# added by ec2hosts installer` to `~/.bashrc` or `~/.zshrc`.
   Unknown shells → warn and skip, no write.
4. **Config seed**: if
   `${XDG_CONFIG_HOME:-$HOME/.config}/ec2hosts/config.yaml` does not
   exist, download `config.yaml.example` from `main` into that path
   (mode 0644). Failure is a warning, not a `die` — the binary is
   already installed and the install is considered successful.
5. **Summary**: binary path, config path, and reload command if a new
   PATH block was appended.

## Verification

Static only — **no automated tests touch `$HOME` or `$XDG_*`** and
`FAKE_HOME`-style sandboxes are forbidden (a previous iteration of this
project was bitten by one).

- `shellcheck install.sh` — must pass clean.
- `bash -n install.sh` — syntax check.
- Manual end-to-end on a real Linux/macOS machine before shipping:
  - re-run after first install leaves `.bashrc` / `.zshrc` unchanged,
  - re-run preserves an existing `config.yaml`,
  - `EC2HOSTS_NO_PATH=1` and `EC2HOSTS_NO_CONFIG=1` each short-circuit
    the relevant step.

## Non-goals

- Windows (already covered by `scripts/build-installer.ps1` + NSIS).
- Package managers (brew / apt / rpm) — out of scope for v1.
- Uninstaller — delete the binary and the marker block manually; could
  ship a `--uninstall` flag later.
