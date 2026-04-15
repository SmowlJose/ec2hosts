# ec2hosts — GUI de Windows (diseño)

Fecha: 2026-04-15
Estado: aprobado, pasa a implementación

## Motivación

El CLI `ec2hosts` resuelve bien el escenario Linux/macOS gracias a la auto-escalada
vía `sudo`. En Windows, hoy el usuario tiene que abrir una PowerShell elevada
cada vez que quiere lanzar `ec2hosts up` o `ec2hosts down`, lo cual añade
fricción diaria a un equipo de ≤5 devs que usa la herramienta en patrón
fire-and-forget (un `up` por la mañana, un `down` por la noche, `status`
ocasional). Queremos una experiencia Windows-nativa con un único punto de
entrada gráfico y un instalador distribuido por GitHub Releases.

## Alcance

**En alcance (v1):**

- Nueva GUI Windows-only basada en Wails v2 + Vue 3.
- UI minimalista: cabecera con estado EC2 + IP, tres botones (Start & apply /
  Stop / Refresh), tabla solo-lectura de hosts configurados, panel de log/error.
- Auto-elevación UAC solo para la escritura del fichero `hosts` (mismo patrón
  que el CLI ya usa con `sudo` en Linux).
- Instalador NSIS generado por Wails, con acceso directo en escritorio y menú
  Inicio.
- GitHub Actions que al tag `v*` publique binarios Linux/macOS + instalador
  Windows en el Release.

**Fuera de alcance (v1):**

- Editor gráfico de `config.yaml` (añadir/quitar hosts, crear targets,
  editar AWS region/profile). El usuario edita el YAML a mano.
- `switch` inline por fila en la GUI. Dado el patrón fire-and-forget no
  compensa. Si en el futuro cambia el patrón se añade.
- Firma de código. ≤5 usuarios internos aceptan el aviso de SmartScreen.
- Bandeja del sistema / systray residente. Proceso siempre corriendo no
  justificado para 2 clicks al día.
- `Restore` y `--dry-run` desde la GUI.
- macOS GUI. Solo Windows.
- Tests automatizados. El repo no tiene tests hoy y no los introducimos en
  v1.
- Manejo de expiración de credenciales AWS (el equipo usa credenciales IAM
  estáticas).

## Usuarios y supuestos

- Equipo ≤5 devs, todos con admin local en Windows (UAC = un click, sin
  contraseña adicional).
- Credenciales AWS vía `~/.aws/credentials` (estáticas, no SSO).
- Windows 10 19041+ o Windows 11 (requisito WebView2 del stack Wails).

## Arquitectura

Monorepo con dos binarios que comparten el núcleo:

```
ec2hosts/
├── cmd/
│   ├── cli/                     # ec2hosts.exe / ec2hosts (CLI actual)
│   │   └── main.go              # movido desde ./main.go
│   └── gui/                     # ec2hosts-gui.exe (nuevo, Windows-only)
│       ├── main.go              # bootstrap Wails
│       ├── app.go               # struct App — métodos expuestos al frontend
│       ├── wails.json           # config Wails (nombre, icono, manifest)
│       ├── build/
│       │   └── windows/
│       │       ├── icon.ico
│       │       ├── info.json
│       │       ├── wails.exe.manifest    # asInvoker
│       │       └── installer/
│       │           └── project.nsi       # tweaks custom (APPDATA, PATH)
│       └── frontend/            # Vue 3 + Vite
│           ├── package.json
│           ├── vite.config.js
│           └── src/
│               ├── App.vue
│               ├── main.js
│               ├── stores/app.js         # Pinia store
│               └── components/
│                   ├── EC2StatusBar.vue
│                   ├── ActionButtons.vue
│                   ├── HostsTable.vue
│                   └── LogPanel.vue
├── internal/
│   ├── awsec2/                  # sin cambios
│   ├── config/                  # sin cambios
│   ├── hosts/                   # sin cambios
│   ├── state/                   # sin cambios
│   └── elevate/                 # NUEVO — extrae lógica de elevación
│       ├── elevate.go           # tipo writeJob + interfaz Elevate
│       ├── elevate_unix.go      # //go:build !windows — sudo
│       └── elevate_windows.go   # //go:build windows — ShellExecuteEx runas
├── go.mod
└── .github/workflows/
    └── release.yml              # construye ambos binarios + installer al tag
```

Claves:

- `internal/` es la única fuente de verdad de la lógica (AWS, parser de
  config, editor de bloque hosts, cache de IPs). Tanto CLI como GUI la
  consumen.
- La GUI tiene su propio `main.go` por exigencia de Wails, pero su
  `app.go` es solo una capa fina: serializa tipos a DTOs y delega en
  `internal/*`.
- El subcomando oculto `__write-hosts` del CLI se mantiene y es reusado
  por la GUI como helper de elevación.
- `cmd/gui` solo compila en Windows mediante `//go:build windows` en su
  `main.go`. El CLI no contamina su build con código de GUI.

## Contrato backend ↔ frontend

Wails v2 genera bindings TS automáticos para los métodos de un struct Go
registrado. En `cmd/gui/app.go`:

```go
type App struct {
    ctx context.Context
    cfg *config.Config
}

// Métodos llamables desde Vue:
func (a *App) Status(ctx context.Context) (StatusDTO, error)
func (a *App) Up(ctx context.Context) (StatusDTO, error)    // = cmdUp
func (a *App) Down(ctx context.Context) error               // = cmdDown
func (a *App) ReadHosts() []HostDTO                         // config + cache
func (a *App) OpenConfigInEditor() error                    // ShellExecute "edit"
func (a *App) OpenConfigFolder() error
```

DTOs (mapeados a TS por Wails):

```go
type StatusDTO struct {
    InstanceID string
    State      string   // "running" | "stopped" | "pending" | …
    PublicIP   string
    UpdatedAt  time.Time
}
type HostDTO struct {
    Host   string
    Target string
    IP     string
}
```

Eventos emitidos con `runtime.EventsEmit` durante operaciones largas
(`Up`, `Down`) para no bloquear la UI:

- `progress` con mensajes tipo "arrancando i-xxx…", "resolviendo IP…",
  "escribiendo hosts…"
- `up:done` / `up:error` / `down:done` / `down:error`

Store Vue (Pinia) con:

- `ec2State: { id, state, ip, updatedAt }`
- `hosts: [{ host, target, ip }, …]`
- `lastLog: [{ level, msg, ts }, …]`
- `busy: boolean` (deshabilita botones durante operaciones)

Al arrancar la app:

1. `main.js` monta la aplicación Vue.
2. `App.vue` dispara `Status()` y `ReadHosts()` en paralelo.
3. Suscribe `runtime.EventsOn('progress', ...)` para alimentar `lastLog`.

Sin polling automático. El estado solo se refresca cuando el usuario pulsa
**Refresh** o ejecuta una acción.

## Privilegios y UAC

La GUI arranca sin elevar (manifest `asInvoker`). Solo elevamos el único
paso que lo requiere: escribir `C:\Windows\System32\drivers\etc\hosts`.

Flujo de "Start & apply":

1. GUI (no-elevada) llama a `internal/awsec2.Start()` y
   `WaitForPublicIP()` con credenciales del usuario. Sin UAC.
2. GUI serializa un `writeJob` a `%TEMP%\ec2hosts-<rand>.json` con ACL del
   usuario.
3. GUI lanza `ec2hosts.exe __write-hosts --job <path>` vía
   `ShellExecuteEx` con `lpVerb="runas"`. Windows muestra UAC. Usuario
   acepta (1 click).
4. El hijo elevado lee el JSON, aplica el cambio y sale con código 0 o 1.
5. La GUI detecta el exit code con `WaitForSingleObject` +
   `GetExitCodeProcess`, borra el fichero temporal, y emite `up:done` o
   `up:error`.

Cambios en el código existente:

- `cmd/cli/main.go` (`__write-hosts`): extender para leer el job desde
  `--job <path>` además de stdin (Windows ShellExecute no puede pasar
  stdin cómodamente al hijo elevado). Mantener stdin para Unix.
- Nuevo paquete `internal/elevate/`:
  - `elevate.go`: tipo `writeJob` (migrado desde `main.go`) + función
    `Run(ctx, job) error`.
  - `elevate_unix.go` (`//go:build !windows`): implementación actual vía
    `sudo`, extraída literalmente del `main.go` actual.
  - `elevate_windows.go` (`//go:build windows`): `ShellExecuteEx` con
    verbo `runas`.
- El CLI pasa a usar este paquete en vez de tener `elevateWrite` inline.
  La GUI también lo usa. Una sola ruta de elevación.

Decisiones explícitas:

- GUI con manifest `asInvoker`, no `requireAdministrator`. Abrir la
  ventana para mirar estado no debe pedir UAC.
- CLI sigue con `asInvoker`. Mismo comportamiento que hoy en la terminal.
- Nada de servicio Windows residente, scheduled tasks con permisos altos,
  ni IPC entre GUI y servicio elevado. Over-engineering para el caso de
  uso (2 clicks/día).
- El fichero temporal del `writeJob` se guarda en claro en `%TEMP%` del
  usuario. Cualquier proceso capaz de escribir ahí ya está dentro de la
  sesión del usuario; cifrarlo no añade seguridad real.

## Instalador y distribución

Wails v2 genera un instalador NSIS oficial con `wails build -nsis`.

El instalador resultante:

- Instala en `%LOCALAPPDATA%\Programs\ec2hosts\`:
  - `ec2hosts-gui.exe` (GUI)
  - `ec2hosts.exe` (CLI, necesario como helper de elevación y como bonus
    para quien quiera usar la terminal)
  - `config.example.yaml`
- Crea acceso directo en el Escritorio y en el menú Inicio → **ec2hosts**.
- Registra desinstalador en "Agregar o quitar programas".
- Detecta WebView2 Runtime y lo descarga si falta (flag
  `-webview2 download` de `wails build`). Binario más pequeño que
  `-webview2 embed`; Win10 19041+/Win11 ya lo tienen la mayoría.

Tweaks custom en `cmd/gui/build/windows/installer/project.nsi`:

- Crear `%APPDATA%\ec2hosts\` al instalar y copiar
  `config.example.yaml` como `config.yaml` **solo si no existe**. No
  pisar config del usuario en reinstalaciones.
- Añadir `%LOCALAPPDATA%\Programs\ec2hosts\` al `PATH` de usuario, para
  que `ec2hosts` sea invocable desde cualquier terminal.

Resolución de `config.yaml` en la GUI: reutiliza la función actual del
CLI (`resolveConfigPath`), que ya busca `./config.yaml` y luego
`os.UserConfigDir()/ec2hosts/config.yaml` (en Windows =
`%APPDATA%\ec2hosts\config.yaml`). El instalador deja el YAML donde el
código ya lo busca — cero configuración manual.

Primer arranque sin `~/.aws/credentials` o sin `config.yaml`: la GUI
pinta un mensaje claro con un botón "Abrir carpeta de configuración" o
"Abrir terminal para `aws configure`". No intentamos asistir el flujo
`aws configure` desde la GUI.

Firma de código: sin firmar en v1. La primera descarga mostrará el
aviso SmartScreen; se salta con "Más información" → "Ejecutar de todas
formas". Si en el futuro molesta, se añade firma tocando solo el
workflow de release — no el diseño.

## GitHub Actions

Un único workflow `.github/workflows/release.yml` disparado por push de
tag `v*`:

- **Job `cli-build`** (runner `ubuntu-latest`): cross-compila el CLI
  para `linux/amd64`, `linux/arm64`, `darwin/amd64`, `darwin/arm64`,
  `windows/amd64`. Sube los binarios como artefactos.
- **Job `gui-build`** (runner `windows-latest`): instala Go + Node +
  Wails CLI, ejecuta `wails build -platform windows/amd64 -nsis` desde
  `cmd/gui/`. Sube `ec2hosts-gui-<version>-amd64-installer.exe` como
  artefacto.
- **Job `release`** (runner `ubuntu-latest`, depende de los dos
  anteriores): descarga todos los artefactos y los sube al GitHub
  Release correspondiente al tag usando `softprops/action-gh-release`.

## Testing

Sin tests automatizados en v1. El repo no tiene tests hoy y no los
introducimos en esta iteración.

Smoke checklist manual en `cmd/gui/TESTING.md`, para quien vaya a
publicar un release:

1. Instalar el `.exe` en Windows 11 limpio (idealmente VM sin WebView2
   para validar la descarga).
2. Primer arranque sin `config.yaml` → mensaje claro + botón "abrir
   carpeta".
3. Primer arranque sin credenciales AWS → mensaje claro.
4. **Start & apply** completo → UAC prompt, entradas añadidas a `hosts`
   con las IPs correctas.
5. **Refresh** tras `Start` → estado `running` visible.
6. **Stop** → UAC prompt, instancia se apaga.
7. Desinstalar → sin residuos salvo `%APPDATA%\ec2hosts\config.yaml`
   (que se preserva a propósito, contiene config del usuario).

## Riesgos y mitigaciones

- **WebView2 ausente en máquinas muy antiguas (<Win10 19041):** el
  instalador lo descarga. Mitigación adicional: documentar el requisito
  mínimo en el README.
- **Usuario con antivirus paranoico bloquea el `runas`:** caso esquina,
  para ≤5 devs internos es fácil resolver ad-hoc. Si afecta a más,
  firmamos el binario.
- **Divergencia entre CLI y GUI:** mitigada por `internal/` compartido
  y por el paquete `internal/elevate/` único.
- **Fichero temporal de `writeJob` queda huérfano si la GUI muere entre
  el write del JSON y el exit del hijo elevado:** añadir cleanup por TTL
  al arrancar (borrar `ec2hosts-*.json` con mtime > 1h en `%TEMP%`).
