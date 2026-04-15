package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"sort"
	"syscall"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/SmowlJose/ec2hosts/internal/awsec2"
	"github.com/SmowlJose/ec2hosts/internal/config"
	"github.com/SmowlJose/ec2hosts/internal/elevate"
	"github.com/SmowlJose/ec2hosts/internal/hosts"
	"github.com/SmowlJose/ec2hosts/internal/state"
)

const usage = `ec2hosts — route hostnames to EC2 or to a local/static IP.

Usage:
  ec2hosts [--config PATH] <command> [flags]

Commands:
  up              Start every EC2 instance referenced by the config and
                  write the hosts file.
  apply           Write the hosts file using cached IPs (and live-resolve
                  anything missing). Does not start/stop instances.
  status          Show the current EC2 state and the hosts block.
  down            Stop every EC2 instance referenced by the config.
  switch HOST T   Rewrite the config so HOST points to target T.
  restore         Remove the managed block from the hosts file.

Global flags:
  --config PATH   Config file (default: ./config.yaml, then
                  $XDG_CONFIG_HOME/ec2hosts/config.yaml).
  --dry-run       Print what would change, do not write the hosts file.
  -h, --help      Show this help.
`

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(argv []string) error {
	// Privilege-separated hosts write step. The parent runs as the user
	// (so AWS credentials resolve cleanly) and re-execs itself via the
	// platform's elevation mechanism only when it needs root/Administrator
	// to touch the hosts file. The actual work happens in elevate.RunChild.
	if len(argv) >= 1 && argv[0] == "__write-hosts" {
		return elevate.RunChild(argv[1:])
	}

	var (
		cfgPath string
		dryRun  bool
		help    bool
	)
	fs := flag.NewFlagSet("ec2hosts", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.StringVar(&cfgPath, "config", "", "path to config.yaml")
	fs.BoolVar(&dryRun, "dry-run", false, "print changes without touching the hosts file")
	fs.BoolVar(&help, "help", false, "show help")
	fs.BoolVar(&help, "h", false, "show help")
	fs.Usage = func() { fmt.Fprint(os.Stderr, usage) }

	if err := fs.Parse(argv); err != nil {
		return err
	}
	if help || fs.NArg() == 0 {
		fmt.Print(usage)
		return nil
	}

	// Best-effort cleanup of stale elevation temp files on every run.
	elevate.CleanupStaleJobs()

	resolved, err := resolveConfigPath(cfgPath)
	if err != nil {
		return err
	}
	cfg, err := config.Load(resolved)
	if err != nil {
		return err
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	cmd := fs.Arg(0)
	args := fs.Args()[1:]
	switch cmd {
	case "up":
		return cmdUp(ctx, cfg, dryRun)
	case "apply":
		return cmdApply(ctx, cfg, dryRun)
	case "status":
		return cmdStatus(ctx, cfg)
	case "down":
		return cmdDown(ctx, cfg)
	case "switch":
		return cmdSwitch(resolved, cfg, args)
	case "restore":
		return cmdRestore(cfg, dryRun)
	default:
		fmt.Fprint(os.Stderr, usage)
		return fmt.Errorf("unknown command %q", cmd)
	}
}

func resolveConfigPath(flagVal string) (string, error) {
	if flagVal != "" {
		return flagVal, nil
	}
	candidates := []string{"config.yaml"}
	if home, err := os.UserConfigDir(); err == nil {
		candidates = append(candidates, home+"/ec2hosts/config.yaml")
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c, nil
		}
	}
	return "", fmt.Errorf("no config found (tried: %v) — pass --config PATH", candidates)
}

// ---- commands ----------------------------------------------------------

func cmdUp(ctx context.Context, cfg *config.Config, dryRun bool) error {
	cli, err := awsec2.New(ctx, cfg.AWS.Region, cfg.AWS.Profile)
	if err != nil {
		return err
	}
	ips, err := resolveTargetIPs(ctx, cli, cfg, true /* startIfStopped */)
	if err != nil {
		return err
	}
	return writeHosts(cfg, ips, dryRun)
}

func cmdApply(ctx context.Context, cfg *config.Config, dryRun bool) error {
	// Try cache first; only hit AWS if something is missing.
	ips, missing := loadCachedIPs(cfg)
	if len(missing) > 0 {
		cli, err := awsec2.New(ctx, cfg.AWS.Region, cfg.AWS.Profile)
		if err != nil {
			return err
		}
		fresh, err := resolveTargetIPs(ctx, cli, cfg, false /* don't start */)
		if err != nil {
			return err
		}
		for k, v := range fresh {
			ips[k] = v
		}
	}
	return writeHosts(cfg, ips, dryRun)
}

func cmdStatus(ctx context.Context, cfg *config.Config) error {
	// EC2 section
	ids := cfg.EC2InstanceIDs()
	if len(ids) > 0 {
		cli, err := awsec2.New(ctx, cfg.AWS.Region, cfg.AWS.Profile)
		if err != nil {
			return err
		}
		fmt.Println("EC2 instances:")
		for _, id := range ids {
			s, err := cli.Describe(ctx, id)
			if err != nil {
				fmt.Printf("  %s  ERROR: %v\n", id, err)
				continue
			}
			ip := s.PublicIP
			if ip == "" {
				ip = "-"
			}
			fmt.Printf("  %s  state=%s  public_ip=%s\n", id, s.State, ip)
		}
	}

	// Hosts block section
	f := hosts.File{Path: cfg.HostsFile, Tag: cfg.MarkerTag}
	entries, err := f.Read()
	if err != nil {
		return fmt.Errorf("read hosts: %w", err)
	}
	fmt.Printf("\nHosts block in %s (tag=%s):\n", cfg.HostsFile, cfg.MarkerTag)
	if len(entries) == 0 {
		fmt.Println("  (empty)")
		return nil
	}
	for _, e := range entries {
		fmt.Printf("  %-15s  %s\n", e.IP, e.Host)
	}
	return nil
}

func cmdDown(ctx context.Context, cfg *config.Config) error {
	ids := cfg.EC2InstanceIDs()
	if len(ids) == 0 {
		fmt.Println("no EC2 targets referenced by any host; nothing to stop")
		return nil
	}
	cli, err := awsec2.New(ctx, cfg.AWS.Region, cfg.AWS.Profile)
	if err != nil {
		return err
	}
	for _, id := range ids {
		fmt.Printf("stopping %s... ", id)
		if err := cli.Stop(ctx, id); err != nil {
			fmt.Println("ERROR")
			return err
		}
		fmt.Println("ok")
	}
	return nil
}

func cmdSwitch(cfgPath string, cfg *config.Config, args []string) error {
	if len(args) != 2 {
		return errors.New("usage: ec2hosts switch <host> <target>")
	}
	host, target := args[0], args[1]
	if _, ok := cfg.Targets[target]; !ok {
		return fmt.Errorf("target %q is not declared in config", target)
	}
	found := false
	for i := range cfg.Hosts {
		if cfg.Hosts[i].Host == host {
			cfg.Hosts[i].Target = target
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("host %q is not declared in config", host)
	}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	if err := os.WriteFile(cfgPath, data, 0o644); err != nil {
		return err
	}
	fmt.Printf("%s → %s (saved in %s)\n", host, target, cfgPath)
	fmt.Println("run `ec2hosts apply` to propagate to the hosts file.")
	return nil
}

func cmdRestore(cfg *config.Config, dryRun bool) error {
	f := hosts.File{Path: cfg.HostsFile, Tag: cfg.MarkerTag}

	// Dry-run is read-only — run in-process regardless of permissions.
	if dryRun {
		changed, err := f.Remove(true)
		if err != nil {
			return err
		}
		if changed {
			fmt.Println("managed block would be removed (dry-run)")
		} else {
			fmt.Println("no managed block found, nothing to do")
		}
		return nil
	}

	changed, err := f.Remove(false)
	if err == nil {
		if changed {
			fmt.Printf("managed block removed from %s\n", cfg.HostsFile)
		} else {
			fmt.Println("no managed block found, nothing to do")
		}
		return nil
	}
	if !elevate.ShouldElevate(err) {
		return err
	}
	// A permission error at write time implies the block existed and
	// will be removed by the elevated child. Announce the final state
	// from the parent since the child is intentionally silent.
	fmt.Fprintln(os.Stderr, "hosts file not writable as user, escalating privileges...")
	if err := elevate.Run(elevate.WriteJob{
		Path:   cfg.HostsFile,
		Tag:    cfg.MarkerTag,
		Remove: true,
	}); err != nil {
		return err
	}
	fmt.Printf("managed block removed from %s\n", cfg.HostsFile)
	return nil
}

// ---- helpers -----------------------------------------------------------

// resolveTargetIPs returns a map target-name -> IP for every target that is
// actually referenced by some host. EC2 targets are resolved via AWS; if
// startIfStopped is true, stopped instances are booted first.
func resolveTargetIPs(ctx context.Context, cli *awsec2.Client, cfg *config.Config, startIfStopped bool) (map[string]string, error) {
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
				fmt.Printf("starting %s (target=%s)... ", t.InstanceID, name)
				if err := cli.Start(ctx, t.InstanceID); err != nil {
					fmt.Println("ERROR")
					return nil, err
				}
				fmt.Println("running")
			}
			fmt.Printf("resolving public IP for %s... ", t.InstanceID)
			ip, err := cli.WaitForPublicIP(ctx, t.InstanceID, 60*time.Second)
			if err != nil {
				fmt.Println("ERROR")
				return nil, err
			}
			fmt.Println(ip)
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
		_ = st.Save(stPath) // best-effort cache; not fatal
	}
	return ips, nil
}

// loadCachedIPs returns (cached-ips, missing-target-names).
func loadCachedIPs(cfg *config.Config) (map[string]string, []string) {
	used := map[string]bool{}
	for _, h := range cfg.Hosts {
		used[cfg.HostTarget(h)] = true
	}
	ips := map[string]string{}
	st, _ := loadState()
	for name := range used {
		t := cfg.Targets[name]
		if t.Type == config.TargetStatic {
			ips[name] = t.IP
			continue
		}
		if st != nil {
			if e, ok := st.Targets[name]; ok && e.InstanceID == t.InstanceID && e.PublicIP != "" {
				ips[name] = e.PublicIP
			}
		}
	}
	var missing []string
	for name := range used {
		if _, ok := ips[name]; !ok {
			missing = append(missing, name)
		}
	}
	sort.Strings(missing)
	return ips, missing
}

func loadState() (*state.State, error) {
	p, err := state.Path()
	if err != nil {
		return nil, err
	}
	return state.Load(p)
}

// writeHosts renders and applies the hosts file from the resolved IPs.
// If the user cannot write the hosts file, the actual write is performed
// by re-execing via the platform's elevation mechanism (elevate.Run).
func writeHosts(cfg *config.Config, ips map[string]string, dryRun bool) error {
	entries := make([]hosts.Entry, 0, len(cfg.Hosts))
	for _, h := range cfg.Hosts {
		ip, ok := ips[cfg.HostTarget(h)]
		if !ok || ip == "" {
			return fmt.Errorf("no IP resolved for host %s (target=%s)", h.Host, cfg.HostTarget(h))
		}
		entries = append(entries, hosts.Entry{IP: ip, Host: h.Host})
	}
	f := hosts.File{Path: cfg.HostsFile, Tag: cfg.MarkerTag}

	// Dry-run is read-only — run in-process regardless of permissions.
	if dryRun {
		content, _, err := f.Apply(entries, true)
		if err != nil {
			return err
		}
		fmt.Println("---- dry run: proposed hosts file ----")
		os.Stdout.Write(content)
		fmt.Println("---- end dry run ----")
		return nil
	}

	_, changed, err := f.Apply(entries, false)
	if err == nil {
		reportApply(changed, cfg.HostsFile, len(entries))
		return nil
	}
	if !elevate.ShouldElevate(err) {
		return err
	}
	// A permission error at write time means Apply already decided there
	// was a change to make; the elevated child will apply it. Announce
	// the final state from the parent since the child is silent.
	fmt.Fprintln(os.Stderr, "hosts file not writable as user, escalating privileges...")
	if err := elevate.Run(elevate.WriteJob{
		Path:    cfg.HostsFile,
		Tag:     cfg.MarkerTag,
		Entries: entries,
	}); err != nil {
		return err
	}
	reportApply(true, cfg.HostsFile, len(entries))
	return nil
}

func reportApply(changed bool, path string, n int) {
	if changed {
		fmt.Printf("%s updated (%d entries)\n", path, n)
	} else {
		fmt.Println("hosts file already up to date")
	}
}
