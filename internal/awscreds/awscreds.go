package awscreds

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// CredentialsPath returns the location of the AWS shared credentials file
// for the current user, honoring AWS_SHARED_CREDENTIALS_FILE when set.
// On Unix that is $HOME/.aws/credentials; on Windows %USERPROFILE%\.aws\credentials.
// We do not create the file or directory — see Save for that.
func CredentialsPath() string {
	if p := os.Getenv("AWS_SHARED_CREDENTIALS_FILE"); p != "" {
		return p
	}
	return filepath.Join(awsDir(), "credentials")
}

// ConfigPath returns the location of the AWS config file, honoring
// AWS_CONFIG_FILE. Same directory as CredentialsPath by default.
func ConfigPath() string {
	if p := os.Getenv("AWS_CONFIG_FILE"); p != "" {
		return p
	}
	return filepath.Join(awsDir(), "config")
}

func awsDir() string {
	// The SDK and CLI both look at USERPROFILE first on Windows, not
	// HOME — matching their behavior avoids split-brain credentials.
	if runtime.GOOS == "windows" {
		if up := os.Getenv("USERPROFILE"); up != "" {
			return filepath.Join(up, ".aws")
		}
	}
	if h, err := os.UserHomeDir(); err == nil {
		return filepath.Join(h, ".aws")
	}
	return ".aws" // fall back to cwd-relative; better than crashing
}

// Creds is the subset of AWS credential fields we manage. Region is not
// stored in ~/.aws/credentials — it belongs to ~/.aws/config — but we
// accept it here and route each field to the correct file.
type Creds struct {
	Profile         string
	AccessKeyID     string
	SecretAccessKey string
	SessionToken    string // optional, for STS/SSO temp creds
	Region          string // optional, written to ~/.aws/config
}

// Status describes what is currently on disk for a given profile. Used
// by the GUI to pre-fill the dialog and show "Not configured yet" vs
// "Configured as AKIA****ABCD".
type Status struct {
	CredentialsPath     string
	ConfigPath          string
	CredentialsFileOK   bool     // file exists and is readable
	ConfigFileOK        bool     // file exists and is readable
	ProfileExists       bool     // profile section exists in credentials
	Profiles            []string // all profile names found in credentials
	MaskedAccessKeyID   string   // "AKIA****MNOP" when ProfileExists
	HasSecretAccessKey  bool
	HasSessionToken     bool
	Region              string // from ~/.aws/config for this profile
}

// Load reads both files and returns the current status for the given
// profile. It never errors on missing files — that's a normal state we
// report via the boolean flags — but it does return an error on
// unreadable or malformed files.
func Load(profile string) (Status, error) {
	if profile == "" {
		profile = "default"
	}
	s := Status{
		CredentialsPath: CredentialsPath(),
		ConfigPath:      ConfigPath(),
	}

	if data, err := os.ReadFile(s.CredentialsPath); err == nil {
		s.CredentialsFileOK = true
		f := parseIni(data)
		s.Profiles = f.listSections()
		if v, ok := f.get(profile, "aws_access_key_id"); ok && v != "" {
			s.ProfileExists = true
			s.MaskedAccessKeyID = maskKey(v)
		}
		if v, ok := f.get(profile, "aws_secret_access_key"); ok && v != "" {
			s.HasSecretAccessKey = true
		}
		if v, ok := f.get(profile, "aws_session_token"); ok && v != "" {
			s.HasSessionToken = true
		}
	} else if !os.IsNotExist(err) {
		return s, fmt.Errorf("read %s: %w", s.CredentialsPath, err)
	}

	if data, err := os.ReadFile(s.ConfigPath); err == nil {
		s.ConfigFileOK = true
		f := parseIni(data)
		// ~/.aws/config uses "[profile foo]" for non-default profiles,
		// but "[default]" for the default one. Handle both.
		section := "profile " + profile
		if profile == "default" {
			section = "default"
		}
		if v, ok := f.get(section, "region"); ok && v != "" {
			s.Region = v
		}
	} else if !os.IsNotExist(err) {
		return s, fmt.Errorf("read %s: %w", s.ConfigPath, err)
	}

	return s, nil
}

// Save writes the credentials (and optionally region) to the shared
// files atomically, creating the ~/.aws directory if needed. Other
// profiles in the same files are left untouched. Unknown keys inside
// the target profile are preserved verbatim; the three known keys
// (aws_access_key_id, aws_secret_access_key, aws_session_token) are
// upserted. An empty SessionToken removes any existing token so the
// caller can switch from temporary to permanent credentials without
// leaving a stale token behind.
func Save(c Creds) error {
	if err := c.validate(); err != nil {
		return err
	}

	dir := awsDir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create %s: %w", dir, err)
	}

	// credentials file
	credsPath := CredentialsPath()
	credsBytes, err := readOrEmpty(credsPath)
	if err != nil {
		return err
	}
	credsIni := parseIni(credsBytes)
	credsIni.upsert(c.Profile, "aws_access_key_id", c.AccessKeyID)
	credsIni.upsert(c.Profile, "aws_secret_access_key", c.SecretAccessKey)
	if c.SessionToken != "" {
		credsIni.upsert(c.Profile, "aws_session_token", c.SessionToken)
	} else {
		credsIni.remove(c.Profile, "aws_session_token")
	}
	if err := writeAtomic(credsPath, credsIni.bytes(), 0o600); err != nil {
		return fmt.Errorf("write %s: %w", credsPath, err)
	}

	// config file — only touch it if we have a region to set, so users
	// who rely purely on env/region discovery keep their current file.
	if c.Region != "" {
		cfgPath := ConfigPath()
		cfgBytes, err := readOrEmpty(cfgPath)
		if err != nil {
			return err
		}
		cfgIni := parseIni(cfgBytes)
		section := "profile " + c.Profile
		if c.Profile == "default" {
			section = "default"
		}
		cfgIni.upsert(section, "region", c.Region)
		if err := writeAtomic(cfgPath, cfgIni.bytes(), 0o600); err != nil {
			return fmt.Errorf("write %s: %w", cfgPath, err)
		}
	}

	return nil
}

func (c Creds) validate() error {
	if strings.TrimSpace(c.Profile) == "" {
		return errors.New("profile name is required")
	}
	if strings.TrimSpace(c.AccessKeyID) == "" {
		return errors.New("access key ID is required")
	}
	if strings.TrimSpace(c.SecretAccessKey) == "" {
		return errors.New("secret access key is required")
	}
	return nil
}

func readOrEmpty(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	return data, nil
}

// writeAtomic writes data to a temp file in the same directory and then
// renames it into place. The rename is atomic on both POSIX and Windows
// (Go's os.Rename uses MoveFileEx with REPLACE_EXISTING on Windows),
// which means a crashed/killed process never leaves a half-written
// credentials file behind.
func writeAtomic(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".awscreds-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	cleanup := func() { _ = os.Remove(tmpPath) }

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		cleanup()
		return err
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return err
	}
	// chmod is a no-op semantically on Windows (no POSIX mode), but Go
	// returns nil there so we can call it unconditionally.
	if err := os.Chmod(tmpPath, mode); err != nil {
		cleanup()
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		cleanup()
		return err
	}
	return nil
}

// maskKey returns a short fingerprint of an AWS access key ID that is
// safe to render in the UI: prefix + stars + last 4 chars. AWS access
// keys are ~20 chars; shorter inputs fall back to stars-only.
func maskKey(k string) string {
	k = strings.TrimSpace(k)
	if len(k) < 8 {
		return strings.Repeat("*", len(k))
	}
	return k[:4] + strings.Repeat("*", len(k)-8) + k[len(k)-4:]
}
