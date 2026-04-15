// Package hosts edits the system hosts file by managing a single
// delimited block, so existing user entries are never touched.
//
// The block looks like:
//
//	# BEGIN ec2hosts (managed — do not edit)
//	1.2.3.4 admin-local.example.com
//	127.0.0.1 api-local.example.com
//	# END ec2hosts
package hosts

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Entry is one "ip host" line inside the managed block.
type Entry struct {
	IP   string
	Host string
}

// File represents a hosts file plus the tag used for block markers.
type File struct {
	Path string
	Tag  string
}

func (f File) beginMarker() string {
	return fmt.Sprintf("# BEGIN %s (managed — do not edit)", f.Tag)
}

func (f File) endMarker() string { return fmt.Sprintf("# END %s", f.Tag) }

// Apply writes entries into the managed block of the hosts file. The block
// is created if missing, replaced if present. Everything outside the block
// is preserved verbatim. If dryRun is true, the resulting content is
// returned but the file is not written.
func (f File) Apply(entries []Entry, dryRun bool) (content []byte, changed bool, err error) {
	existing, err := os.ReadFile(f.Path)
	if err != nil && !os.IsNotExist(err) {
		return nil, false, fmt.Errorf("read hosts: %w", err)
	}

	stripped := stripBlock(existing, f.beginMarker(), f.endMarker())
	block := renderBlock(entries, f.beginMarker(), f.endMarker())

	var out bytes.Buffer
	out.Write(stripped)
	// Guarantee a newline between user content and our block.
	if stripped != nil && !bytes.HasSuffix(stripped, []byte("\n")) {
		out.WriteByte('\n')
	}
	out.Write(block)

	changed = !bytes.Equal(existing, out.Bytes())
	if dryRun || !changed {
		return out.Bytes(), changed, nil
	}

	if err := f.backup(existing); err != nil {
		return nil, false, err
	}
	if err := writeAtomic(f.Path, out.Bytes()); err != nil {
		return nil, false, err
	}
	return out.Bytes(), true, nil
}

// Remove strips the managed block from the hosts file. Used by `restore`.
func (f File) Remove(dryRun bool) (changed bool, err error) {
	existing, err := os.ReadFile(f.Path)
	if err != nil {
		return false, fmt.Errorf("read hosts: %w", err)
	}
	stripped := stripBlock(existing, f.beginMarker(), f.endMarker())
	if bytes.Equal(stripped, existing) {
		return false, nil
	}
	if dryRun {
		return true, nil
	}
	if err := f.backup(existing); err != nil {
		return false, err
	}
	return true, writeAtomic(f.Path, stripped)
}

// Read returns the current entries inside the managed block, in file order.
func (f File) Read() ([]Entry, error) {
	data, err := os.ReadFile(f.Path)
	if err != nil {
		return nil, err
	}
	_, block, ok := splitBlock(data, f.beginMarker(), f.endMarker())
	if !ok {
		return nil, nil
	}
	var out []Entry
	sc := bufio.NewScanner(strings.NewReader(block))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		out = append(out, Entry{IP: fields[0], Host: fields[1]})
	}
	return out, sc.Err()
}

// --- helpers ------------------------------------------------------------

func renderBlock(entries []Entry, begin, end string) []byte {
	// Stable ordering: group by IP, then alphabetical. Keeps diffs quiet.
	sorted := append([]Entry(nil), entries...)
	sort.SliceStable(sorted, func(i, j int) bool {
		if sorted[i].IP != sorted[j].IP {
			return sorted[i].IP < sorted[j].IP
		}
		return sorted[i].Host < sorted[j].Host
	})

	var b bytes.Buffer
	fmt.Fprintln(&b, begin)
	for _, e := range sorted {
		fmt.Fprintf(&b, "%s %s\n", e.IP, e.Host)
	}
	fmt.Fprintln(&b, end)
	return b.Bytes()
}

// stripBlock returns the file content with any existing managed block
// removed. If no block is found, returns the input unchanged.
func stripBlock(content []byte, begin, end string) []byte {
	before, _, ok := splitBlock(content, begin, end)
	if !ok {
		return content
	}
	// Trim a single trailing blank line we might have introduced.
	return bytes.TrimRight(before, "\n")
}

// splitBlock locates the BEGIN/END markers and returns the content before
// the block and the inner body (without markers). ok=false when no block.
func splitBlock(content []byte, begin, end string) (before []byte, inner string, ok bool) {
	bi := bytes.Index(content, []byte(begin))
	if bi < 0 {
		return content, "", false
	}
	after := content[bi:]
	ei := bytes.Index(after, []byte(end))
	if ei < 0 {
		// Broken block: strip from BEGIN to EOF to be safe.
		return content[:bi], string(after), true
	}
	// include end marker + its trailing newline if present
	blockEnd := bi + ei + len(end)
	if blockEnd < len(content) && content[blockEnd] == '\n' {
		blockEnd++
	}
	inner = string(content[bi+len(begin) : bi+ei])
	return content[:bi], inner, true
}

func (f File) backup(existing []byte) error {
	if existing == nil {
		return nil
	}
	dir := filepath.Dir(f.Path)
	stamp := time.Now().Format("20060102-150405")
	path := filepath.Join(dir, filepath.Base(f.Path)+".bak-"+stamp)
	return os.WriteFile(path, existing, 0o644)
}

// writeAtomic writes via a temp file in the same directory, then renames.
// This avoids leaving a half-written /etc/hosts if the process is killed.
func writeAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".hosts-*")
	if err != nil {
		return fmt.Errorf("create temp: %w", err)
	}
	tmpPath := tmp.Name()
	cleanup := func() { _ = os.Remove(tmpPath) }

	if _, err := io.Copy(tmp, bytes.NewReader(data)); err != nil {
		tmp.Close()
		cleanup()
		return err
	}
	if err := tmp.Chmod(0o644); err != nil {
		tmp.Close()
		cleanup()
		return err
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		cleanup()
		return fmt.Errorf("rename over %s: %w", path, err)
	}
	return nil
}
