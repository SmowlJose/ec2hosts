//go:build windows

package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/SmowlJose/ec2hosts/internal/awsec2"
	"github.com/SmowlJose/ec2hosts/internal/config"
	"github.com/SmowlJose/ec2hosts/internal/elevate"
	"github.com/SmowlJose/ec2hosts/internal/hosts"
	"github.com/SmowlJose/ec2hosts/internal/state"
)

// App is the struct exposed to the Vue frontend via Wails bindings.
// Every exported method becomes a TypeScript-callable function; every
// exported struct becomes a TS type.
type App struct {
	ctx     context.Context
	cfgPath string
}

func NewApp() *App { return &App{} }

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	a.cfgPath, _ = resolveConfigPath()
}

func (a *App) shutdown(ctx context.Context) {}

// ---- DTOs (auto-mapped to TypeScript by Wails) ----

// StatusDTO is the view of a single EC2 instance shown in the UI.
type StatusDTO struct {
	InstanceID string    `json:"instanceId"`
	State      string    `json:"state"` // "running" | "stopped" | "pending" | "error" | ...
	PublicIP   string    `json:"publicIp"`
	UpdatedAt  time.Time `json:"updatedAt"`
}

// HostDTO is one row of the hosts table.
type HostDTO struct {
	Host   string `json:"host"`
	Target string `json:"target"`
	IP     string `json:"ip"`
}

// ConfigInfoDTO describes where the config lives and its basic metadata.
// Used by the UI to show "Open config.yaml" and to detect misconfiguration
// on first run.
type ConfigInfoDTO struct {
	Path        string   `json:"path"`
	Found       bool     `json:"found"`
	Error       string   `json:"error"`
	InstanceIDs []string `json:"instanceIds"`
	AWSRegion   string   `json:"awsRegion"`
	AWSProfile  string   `json:"awsProfile"`
	HostsFile   string   `json:"hostsFile"`
}

// ---- methods bound to the frontend ----

// ConfigInfo returns the current config path + a snapshot of its contents,
// or a structured error if the config is missing or invalid. Used by the
// UI to decide what initial view to render.
func (a *App) ConfigInfo() ConfigInfoDTO {
	info := ConfigInfoDTO{Path: a.cfgPath}
	if a.cfgPath == "" {
		info.Error = "config.yaml not found"
		return info
	}
	cfg, err := config.Load(a.cfgPath)
	if err != nil {
		info.Error = err.Error()
		return info
	}
	info.Found = true
	info.InstanceIDs = cfg.EC2InstanceIDs()
	info.AWSRegion = cfg.AWS.Region
	info.AWSProfile = cfg.AWS.Profile
	info.HostsFile = cfg.HostsFile
	return info
}

// Status describes every EC2 instance referenced by the config.
func (a *App) Status() ([]StatusDTO, error) {
	cfg, err := a.loadConfig()
	if err != nil {
		return nil, err
	}
	cli, err := awsec2.New(a.ctx, cfg.AWS.Region, cfg.AWS.Profile)
	if err != nil {
		return nil, err
	}
	ids := cfg.EC2InstanceIDs()
	out := make([]StatusDTO, 0, len(ids))
	for _, id := range ids {
		s, err := cli.Describe(a.ctx, id)
		if err != nil {
			out = append(out, StatusDTO{InstanceID: id, State: "error", UpdatedAt: time.Now()})
			continue
		}
		out = append(out, StatusDTO{
			InstanceID: id,
			State:      string(s.State),
			PublicIP:   s.PublicIP,
			UpdatedAt:  time.Now(),
		})
	}
	return out, nil
}

// ReadHosts returns the list of configured hosts with their current target
// and last-known IP (from the state cache for EC2, literal for static).
// Does not hit AWS.
func (a *App) ReadHosts() ([]HostDTO, error) {
	cfg, err := a.loadConfig()
	if err != nil {
		return nil, err
	}
	st, _ := loadState()
	out := make([]HostDTO, 0, len(cfg.Hosts))
	for _, h := range cfg.Hosts {
		targetName := cfg.HostTarget(h)
		t := cfg.Targets[targetName]
		ip := ""
		switch t.Type {
		case config.TargetStatic:
			ip = t.IP
		case config.TargetEC2:
			if st != nil {
				if e, ok := st.Targets[targetName]; ok && e.InstanceID == t.InstanceID {
					ip = e.PublicIP
				}
			}
		}
		out = append(out, HostDTO{Host: h.Host, Target: targetName, IP: ip})
	}
	return out, nil
}

// Up is the equivalent of `ec2hosts up`: start every referenced EC2
// instance, resolve IPs, rewrite hosts (via UAC elevation if needed).
// Emits `progress` events for the log panel.
func (a *App) Up() error {
	cfg, err := a.loadConfig()
	if err != nil {
		return err
	}
	cli, err := awsec2.New(a.ctx, cfg.AWS.Region, cfg.AWS.Profile)
	if err != nil {
		return err
	}

	ips, err := a.resolveTargetIPs(cli, cfg, true /* startIfStopped */)
	if err != nil {
		return err
	}

	entries := make([]hosts.Entry, 0, len(cfg.Hosts))
	for _, h := range cfg.Hosts {
		ip, ok := ips[cfg.HostTarget(h)]
		if !ok || ip == "" {
			return fmt.Errorf("no IP resolved for host %s (target=%s)", h.Host, cfg.HostTarget(h))
		}
		entries = append(entries, hosts.Entry{IP: ip, Host: h.Host})
	}

	f := hosts.File{Path: cfg.HostsFile, Tag: cfg.MarkerTag}
	a.emit("progress", "writing hosts file...")
	_, _, err = f.Apply(entries, false)
	if err == nil {
		a.emit("progress", fmt.Sprintf("hosts file updated (%d entries)", len(entries)))
		return nil
	}
	if !elevate.ShouldElevate(err) {
		return err
	}

	a.emit("progress", "requesting UAC elevation to write hosts file...")
	if err := elevate.Run(elevate.WriteJob{
		Path:    cfg.HostsFile,
		Tag:     cfg.MarkerTag,
		Entries: entries,
	}); err != nil {
		return err
	}
	a.emit("progress", fmt.Sprintf("hosts file updated (%d entries)", len(entries)))
	return nil
}

// Down is the equivalent of `ec2hosts down`: stop every referenced EC2
// instance. No hosts-file changes.
func (a *App) Down() error {
	cfg, err := a.loadConfig()
	if err != nil {
		return err
	}
	ids := cfg.EC2InstanceIDs()
	if len(ids) == 0 {
		a.emit("progress", "no EC2 targets referenced by any host; nothing to stop")
		return nil
	}
	cli, err := awsec2.New(a.ctx, cfg.AWS.Region, cfg.AWS.Profile)
	if err != nil {
		return err
	}
	for _, id := range ids {
		a.emit("progress", fmt.Sprintf("stopping %s...", id))
		if err := cli.Stop(a.ctx, id); err != nil {
			return err
		}
	}
	a.emit("progress", "done")
	return nil
}

// OpenConfigInEditor opens config.yaml in the user's default associated
// application (typically notepad or whatever they registered for .yaml).
func (a *App) OpenConfigInEditor() error {
	if a.cfgPath == "" {
		return fmt.Errorf("config.yaml not found — try Open Config Folder first")
	}
	// `cmd /C start "" <file>` launches the file with its default app
	// without opening a lingering cmd window.
	return exec.Command("cmd", "/C", "start", "", a.cfgPath).Start()
}

// OpenConfigFolder opens the directory that contains (or should contain)
// config.yaml in Windows Explorer.
func (a *App) OpenConfigFolder() error {
	path := a.cfgPath
	if path == "" {
		home, err := os.UserConfigDir()
		if err != nil {
			return err
		}
		path = filepath.Join(home, "ec2hosts")
		_ = os.MkdirAll(path, 0o755)
	} else {
		path = filepath.Dir(path)
	}
	return exec.Command("explorer", path).Start()
}

// ---- helpers ----

func (a *App) emit(event string, msg string) {
	if a.ctx != nil {
		runtime.EventsEmit(a.ctx, event, msg)
	}
}

func (a *App) loadConfig() (*config.Config, error) {
	if a.cfgPath == "" {
		return nil, fmt.Errorf("config.yaml not found")
	}
	return config.Load(a.cfgPath)
}

// resolveTargetIPs mirrors the CLI logic: start EC2 instances when
// requested, then resolve their public IPs, then persist to the state
// cache. Emits `progress` events so the UI can show what's happening.
func (a *App) resolveTargetIPs(cli *awsec2.Client, cfg *config.Config, startIfStopped bool) (map[string]string, error) {
	used := map[string]bool{}
	for _, h := range cfg.Hosts {
		used[cfg.HostTarget(h)] = true
	}

	ips := map[string]string{}
	st, _ := loadState()
	stPath, _ := state.Path()

	for name := range used {
		t := cfg.Targets[name]
		switch t.Type {
		case config.TargetStatic:
			ips[name] = t.IP
		case config.TargetEC2:
			if startIfStopped {
				a.emit("progress", fmt.Sprintf("starting %s (target=%s)...", t.InstanceID, name))
				if err := cli.Start(a.ctx, t.InstanceID); err != nil {
					return nil, err
				}
			}
			a.emit("progress", fmt.Sprintf("resolving public IP for %s...", t.InstanceID))
			ip, err := cli.WaitForPublicIP(a.ctx, t.InstanceID, 60*time.Second)
			if err != nil {
				return nil, err
			}
			a.emit("progress", fmt.Sprintf("  → %s = %s", t.InstanceID, ip))
			ips[name] = ip

			if st != nil {
				st.Targets[name] = state.Entry{
					InstanceID: t.InstanceID,
					PublicIP:   ip,
					UpdatedAt:  time.Now(),
				}
			}
		}
	}
	if st != nil && stPath != "" {
		_ = st.Save(stPath)
	}
	return ips, nil
}

func loadState() (*state.State, error) {
	p, err := state.Path()
	if err != nil {
		return nil, err
	}
	return state.Load(p)
}

// resolveConfigPath uses the same search order as the CLI:
// 1. ./config.yaml (next to the binary / current dir)
// 2. %APPDATA%\ec2hosts\config.yaml
func resolveConfigPath() (string, error) {
	candidates := []string{"config.yaml"}
	if home, err := os.UserConfigDir(); err == nil {
		candidates = append(candidates, filepath.Join(home, "ec2hosts", "config.yaml"))
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c, nil
		}
	}
	return "", fmt.Errorf("no config found (tried: %v)", candidates)
}
