// Package config parses the ec2hosts YAML configuration.
//
// The config declares a set of *targets* (named IP sources) and a list of
// *hosts* each bound to one of those targets. A target is either an EC2
// instance (resolved at runtime to its public IP) or a static IP (e.g. the
// loopback address, or a teammate's machine).
package config

import (
	"errors"
	"fmt"
	"net"
	"os"
	"runtime"

	"gopkg.in/yaml.v3"
)

type TargetType string

const (
	TargetEC2    TargetType = "ec2"
	TargetStatic TargetType = "static"
)

type Target struct {
	Type       TargetType `yaml:"type"`
	InstanceID string     `yaml:"instance_id,omitempty"` // ec2 only
	IP         string     `yaml:"ip,omitempty"`          // static only
}

type HostEntry struct {
	Host   string `yaml:"host"`
	Target string `yaml:"target,omitempty"` // falls back to DefaultTarget
}

type AWSConfig struct {
	Region  string `yaml:"region"`
	Profile string `yaml:"profile,omitempty"`
}

type Config struct {
	AWS           AWSConfig         `yaml:"aws"`
	HostsFile     string            `yaml:"hosts_file,omitempty"`
	MarkerTag     string            `yaml:"marker_tag,omitempty"`
	DefaultTarget string            `yaml:"default_target"`
	Targets       map[string]Target `yaml:"targets"`
	Hosts         []HostEntry       `yaml:"hosts"`
}

// Load reads and validates the YAML config at path.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config %s: %w", path, err)
	}
	cfg.applyDefaults()
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func (c *Config) applyDefaults() {
	if c.MarkerTag == "" {
		c.MarkerTag = "ec2hosts"
	}
	if c.HostsFile == "" {
		c.HostsFile = defaultHostsPath()
	}
}

func defaultHostsPath() string {
	if runtime.GOOS == "windows" {
		// %SystemRoot% is usually C:\Windows.
		root := os.Getenv("SystemRoot")
		if root == "" {
			root = `C:\Windows`
		}
		return root + `\System32\drivers\etc\hosts`
	}
	return "/etc/hosts"
}

func (c *Config) validate() error {
	if len(c.Targets) == 0 {
		return errors.New("config: at least one target is required")
	}
	for name, t := range c.Targets {
		switch t.Type {
		case TargetEC2:
			if t.InstanceID == "" {
				return fmt.Errorf("target %q: instance_id is required for ec2 targets", name)
			}
		case TargetStatic:
			if t.IP == "" {
				return fmt.Errorf("target %q: ip is required for static targets", name)
			}
			if net.ParseIP(t.IP) == nil {
				return fmt.Errorf("target %q: %q is not a valid IP", name, t.IP)
			}
		default:
			return fmt.Errorf("target %q: unknown type %q (want ec2|static)", name, t.Type)
		}
	}
	if c.DefaultTarget != "" {
		if _, ok := c.Targets[c.DefaultTarget]; !ok {
			return fmt.Errorf("default_target %q is not declared in targets", c.DefaultTarget)
		}
	}
	seen := make(map[string]bool, len(c.Hosts))
	for i, h := range c.Hosts {
		if h.Host == "" {
			return fmt.Errorf("hosts[%d]: empty host", i)
		}
		if seen[h.Host] {
			return fmt.Errorf("hosts[%d]: duplicate entry for %q", i, h.Host)
		}
		seen[h.Host] = true

		target := h.Target
		if target == "" {
			target = c.DefaultTarget
		}
		if target == "" {
			return fmt.Errorf("hosts[%d] (%s): no target and no default_target set", i, h.Host)
		}
		if _, ok := c.Targets[target]; !ok {
			return fmt.Errorf("hosts[%d] (%s): target %q is not declared", i, h.Host, target)
		}
	}
	return nil
}

// HostTarget returns the effective target name for a host entry, applying
// the default if the entry does not specify one.
func (c *Config) HostTarget(h HostEntry) string {
	if h.Target != "" {
		return h.Target
	}
	return c.DefaultTarget
}

// EC2InstanceIDs returns every distinct EC2 instance ID referenced by a host
// that is currently routed to an EC2 target.
func (c *Config) EC2InstanceIDs() []string {
	seen := map[string]bool{}
	var out []string
	for _, h := range c.Hosts {
		t := c.Targets[c.HostTarget(h)]
		if t.Type == TargetEC2 && !seen[t.InstanceID] {
			seen[t.InstanceID] = true
			out = append(out, t.InstanceID)
		}
	}
	return out
}
