// Package awscreds reads and writes the AWS shared credentials and config
// files (~/.aws/credentials and ~/.aws/config). It preserves unrelated
// sections verbatim so users who have already configured other tools
// (terraform, the real AWS CLI, SSO profiles) do not lose data when the
// GUI updates a single profile.
package awscreds

import (
	"bytes"
	"strings"
)

// iniFile is a tiny line-preserving INI model. The goal is NOT to be a
// general INI editor — it is to round-trip the AWS shared credentials
// format (simple [section] + key = value, no nested structures) while
// keeping comments, blank lines, and unknown keys untouched.
//
// The AWS CLI actually accepts a few deviations from classic INI
// (profile names like "[profile foo]" in ~/.aws/config); those are
// preserved as-is because we treat section headers as opaque strings.
type iniFile struct {
	lines []iniLine
}

// iniLine keeps the original bytes so round-tripping is byte-exact when
// we don't touch a section. For section/kv lines we also keep parsed
// fields for fast lookups.
type iniLine struct {
	raw     string // original line WITHOUT the trailing \n
	kind    lineKind
	section string // non-empty only when kind == lineSection
	key     string // non-empty only when kind == lineKV
	value   string // non-empty only when kind == lineKV
}

type lineKind int

const (
	lineBlank lineKind = iota
	lineComment
	lineSection
	lineKV
)

// parseIni is lenient: any line that doesn't match [section] or key=value
// is kept as-is under the kind=lineBlank/lineComment bucket. We never
// lose bytes.
func parseIni(data []byte) *iniFile {
	f := &iniFile{}
	// Preserve trailing-newline semantics: split on \n, then strip \r
	// from each element. Writing always re-joins with \n.
	for _, raw := range strings.Split(string(data), "\n") {
		line := strings.TrimRight(raw, "\r")
		f.lines = append(f.lines, classifyLine(line))
	}
	// Split on "\n" on input "a\nb\n" yields ["a","b",""] — the trailing
	// empty string is a blank line that we want to keep so writes end in
	// a newline. No special handling needed.
	return f
}

func classifyLine(raw string) iniLine {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return iniLine{raw: raw, kind: lineBlank}
	}
	if trimmed[0] == '#' || trimmed[0] == ';' {
		return iniLine{raw: raw, kind: lineComment}
	}
	if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
		return iniLine{
			raw:     raw,
			kind:    lineSection,
			section: strings.TrimSpace(trimmed[1 : len(trimmed)-1]),
		}
	}
	if i := strings.IndexByte(trimmed, '='); i >= 0 {
		return iniLine{
			raw:   raw,
			kind:  lineKV,
			key:   strings.TrimSpace(trimmed[:i]),
			value: strings.TrimSpace(trimmed[i+1:]),
		}
	}
	// Unrecognized (e.g. continuation lines from pathological edits) —
	// classify as comment so we preserve it untouched.
	return iniLine{raw: raw, kind: lineComment}
}

// sectionRange returns [headerIdx, nextHeaderIdx) for the given section,
// or (-1, -1) if it doesn't exist. Header is inclusive, next header is
// exclusive — so body lines are in (header, next).
func (f *iniFile) sectionRange(name string) (int, int) {
	headerIdx := -1
	for i, l := range f.lines {
		if l.kind == lineSection && l.section == name {
			headerIdx = i
			break
		}
	}
	if headerIdx < 0 {
		return -1, -1
	}
	next := len(f.lines)
	for i := headerIdx + 1; i < len(f.lines); i++ {
		if f.lines[i].kind == lineSection {
			next = i
			break
		}
	}
	return headerIdx, next
}

// listSections returns section names in the order they appear.
func (f *iniFile) listSections() []string {
	var out []string
	for _, l := range f.lines {
		if l.kind == lineSection {
			out = append(out, l.section)
		}
	}
	return out
}

// get returns the value of key inside section, plus whether it was found.
// The first matching key wins (the AWS CLI also uses first-write-wins,
// so this matches reality).
func (f *iniFile) get(section, key string) (string, bool) {
	start, end := f.sectionRange(section)
	if start < 0 {
		return "", false
	}
	for i := start + 1; i < end; i++ {
		if f.lines[i].kind == lineKV && f.lines[i].key == key {
			return f.lines[i].value, true
		}
	}
	return "", false
}

// upsert inserts or updates a key inside a section. The section is
// created (at EOF) if it does not exist. When replacing, the line's
// formatting (whitespace around `=`) is normalized to `key = value`;
// we accept that minor churn in exchange for not trying to preserve
// arbitrary original formatting.
func (f *iniFile) upsert(section, key, value string) {
	start, end := f.sectionRange(section)
	if start < 0 {
		// Append section at EOF. Ensure there's a blank line before
		// it if the previous line isn't blank already — keeps the
		// output readable.
		if n := len(f.lines); n > 0 && f.lines[n-1].kind != lineBlank {
			f.lines = append(f.lines, iniLine{raw: "", kind: lineBlank})
		}
		f.lines = append(f.lines,
			iniLine{raw: "[" + section + "]", kind: lineSection, section: section},
			iniLine{raw: key + " = " + value, kind: lineKV, key: key, value: value},
		)
		return
	}
	// Update existing key in-place if present.
	for i := start + 1; i < end; i++ {
		if f.lines[i].kind == lineKV && f.lines[i].key == key {
			f.lines[i].value = value
			f.lines[i].raw = key + " = " + value
			return
		}
	}
	// Not present — insert right before the next section (or before the
	// tail blank lines at EOF). We skip trailing blank lines inside the
	// section so the new key appears next to the other keys.
	insertAt := end
	for insertAt > start+1 && f.lines[insertAt-1].kind == lineBlank {
		insertAt--
	}
	newLine := iniLine{raw: key + " = " + value, kind: lineKV, key: key, value: value}
	f.lines = append(f.lines[:insertAt], append([]iniLine{newLine}, f.lines[insertAt:]...)...)
}

// remove deletes a key inside a section, if present. No-op otherwise.
func (f *iniFile) remove(section, key string) {
	start, end := f.sectionRange(section)
	if start < 0 {
		return
	}
	for i := start + 1; i < end; i++ {
		if f.lines[i].kind == lineKV && f.lines[i].key == key {
			f.lines = append(f.lines[:i], f.lines[i+1:]...)
			return
		}
	}
}

// bytes returns the file as it would be written to disk. Lines are
// joined with '\n'; we don't emit '\r\n' even on Windows because the
// AWS CLI reads both fine and keeping one canonical form makes diffs
// easier to read.
func (f *iniFile) bytes() []byte {
	var buf bytes.Buffer
	for i, l := range f.lines {
		if i > 0 {
			buf.WriteByte('\n')
		}
		buf.WriteString(l.raw)
	}
	return buf.Bytes()
}
