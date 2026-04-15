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
// and returns any error.
//
// When invoked via UAC on Windows the child's stdout/stderr is NOT
// captured by the parent (ShellExecuteEx is fire-and-forget with just
// an exit code), so a bare error message would vanish. To work around
// that, if the caller passed --job <path>, we also write any error
// text to a sibling file "<path>.err" before returning. The parent
// reads that file when the exit code is non-zero and surfaces the
// real cause in the UI. On success the .err file is removed so stale
// messages from previous failed runs can't mislead us.
func RunChild(args []string) error {
	fs := flag.NewFlagSet("__write-hosts", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	jobPath := fs.String("job", "", "path to a WriteJob JSON file (defaults to stdin)")
	if err := fs.Parse(args); err != nil {
		persistChildError(*jobPath, err)
		return err
	}
	err := runChildInner(*jobPath)
	persistChildError(*jobPath, err) // nil removes the sidecar
	return err
}

func runChildInner(jobPath string) error {
	var r io.Reader = os.Stdin
	if jobPath != "" {
		f, err := os.Open(jobPath)
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

// persistChildError writes the error text to <jobPath>.err so the
// unelevated parent can read it after we exit. Called with err=nil on
// success to clear any sidecar left by a previous failing run, so a
// stale .err from an older process never gets attributed to a fresh
// successful one.
func persistChildError(jobPath string, err error) {
	if jobPath == "" {
		return
	}
	sidecar := jobPath + ".err"
	if err == nil {
		_ = os.Remove(sidecar)
		return
	}
	_ = os.WriteFile(sidecar, []byte(err.Error()), 0o600)
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
