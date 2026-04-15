//go:build !windows

package elevate

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
)

// ShouldElevate reports whether err is worth retrying under elevation.
// True only for permission errors when not already running as root.
func ShouldElevate(err error) bool {
	if !os.IsPermission(err) {
		return false
	}
	return os.Geteuid() != 0
}

// Run re-executes the current binary under sudo to perform the job.
// Stdin carries the JSON job; sudo reads the password from /dev/tty, so
// there is no clash between the password prompt and the JSON pipe.
func Run(job WriteJob) error {
	self, err := os.Executable()
	if err != nil {
		return fmt.Errorf("locate self: %w", err)
	}
	payload, err := json.Marshal(job)
	if err != nil {
		return err
	}
	cmd := exec.Command("sudo", "--", self, "__write-hosts")
	cmd.Stdin = bytes.NewReader(payload)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// CleanupStaleJobs is a no-op on Unix — the job is piped over stdin,
// so no temp files are left behind.
func CleanupStaleJobs() {}
