// Package elevate re-executes the current binary with elevated privileges
// to perform a single narrow task: write the managed block into the
// system hosts file (or remove it). The parent process runs as the user
// — which keeps AWS credentials resolvable — and only this single step
// crosses the privilege boundary.
//
// On Unix the elevation is done via `sudo`; on Windows via the UAC
// prompt (ShellExecuteEx with verb "runas"). Both flavors are wired
// from the main binary's hidden `__write-hosts` subcommand, which
// delegates to RunChild below.
package elevate

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/SmowlJose/ec2hosts/internal/hosts"
)

// WriteJob is the payload the parent sends to the privileged child.
// Keep fields stable; this is an internal ABI between two copies of
// the same binary.
type WriteJob struct {
	Path    string        `json:"path"`
	Tag     string        `json:"tag"`
	Entries []hosts.Entry `json:"entries,omitempty"`
	Remove  bool          `json:"remove,omitempty"`
}

// RunChild is the entry point of the privileged child process. It reads
// a WriteJob (from --job <path> if provided, else stdin), applies it,
// and returns any error. Silent on success — the parent prints the
// final status after detecting a zero exit code.
func RunChild(args []string) error {
	fs := flag.NewFlagSet("__write-hosts", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	jobPath := fs.String("job", "", "path to a WriteJob JSON file (defaults to stdin)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	var r io.Reader = os.Stdin
	if *jobPath != "" {
		f, err := os.Open(*jobPath)
		if err != nil {
			return fmt.Errorf("open job file: %w", err)
		}
		defer f.Close()
		r = f
	}

	var job WriteJob
	if err := json.NewDecoder(r).Decode(&job); err != nil {
		return fmt.Errorf("decode write job: %w", err)
	}
	return performJob(job)
}

// performJob applies or removes the managed block as described by job.
func performJob(job WriteJob) error {
	f := hosts.File{Path: job.Path, Tag: job.Tag}
	if job.Remove {
		_, err := f.Remove(false)
		return err
	}
	_, _, err := f.Apply(job.Entries, false)
	return err
}
