//go:build windows

package elevate

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Win32 bindings for ShellExecuteExW. golang.org/x/sys/windows exposes
// plenty of helpers, but not this one — so we declare the lazy-loaded
// proc and mirror the SHELLEXECUTEINFOW struct verbatim.
var (
	modShell32         = windows.NewLazySystemDLL("shell32.dll")
	procShellExecuteEx = modShell32.NewProc("ShellExecuteExW")
)

const (
	seeMaskNoCloseProcess = 0x00000040
	swShowNormal          = 1
)

type shellExecuteInfo struct {
	cbSize       uint32
	fMask        uint32
	hwnd         windows.Handle
	lpVerb       *uint16
	lpFile       *uint16
	lpParameters *uint16
	lpDirectory  *uint16
	nShow        int32
	hInstApp     windows.Handle
	lpIDList     uintptr
	lpClass      *uint16
	hkeyClass    windows.Handle
	dwHotKey     uint32
	hIcon        windows.Handle // also serves as hMonitor (union in the C struct)
	hProcess     windows.Handle
}

// ShouldElevate reports whether err is worth retrying under UAC.
// On Windows any permission error is potentially fixable by elevating.
func ShouldElevate(err error) bool {
	return os.IsPermission(err)
}

// Run re-executes the current binary under UAC to perform the job.
// ShellExecuteEx's "runas" verb triggers the UAC prompt. The job is
// passed via a temp JSON file instead of stdin because stdin cannot
// be piped into an elevated child under this API.
func Run(job WriteJob) error {
	self, err := os.Executable()
	if err != nil {
		return fmt.Errorf("locate self: %w", err)
	}
	payload, err := json.Marshal(job)
	if err != nil {
		return err
	}

	tmp, err := os.CreateTemp("", "ec2hosts-job-*.json")
	if err != nil {
		return fmt.Errorf("create temp: %w", err)
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(payload); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return err
	}
	defer os.Remove(tmpPath)

	verbPtr, err := syscall.UTF16PtrFromString("runas")
	if err != nil {
		return err
	}
	exePtr, err := syscall.UTF16PtrFromString(self)
	if err != nil {
		return err
	}
	args := fmt.Sprintf(`__write-hosts --job %s`, quoteArg(tmpPath))
	argsPtr, err := syscall.UTF16PtrFromString(args)
	if err != nil {
		return err
	}

	info := shellExecuteInfo{
		fMask:        seeMaskNoCloseProcess,
		lpVerb:       verbPtr,
		lpFile:       exePtr,
		lpParameters: argsPtr,
		nShow:        swShowNormal,
	}
	info.cbSize = uint32(unsafe.Sizeof(info))

	ret, _, callErr := procShellExecuteEx.Call(uintptr(unsafe.Pointer(&info)))
	if ret == 0 {
		return fmt.Errorf("ShellExecuteEx failed: %w", callErr)
	}
	defer windows.CloseHandle(info.hProcess)

	if _, err := windows.WaitForSingleObject(info.hProcess, windows.INFINITE); err != nil {
		return fmt.Errorf("wait for elevated child: %w", err)
	}
	var code uint32
	if err := windows.GetExitCodeProcess(info.hProcess, &code); err != nil {
		return fmt.Errorf("get child exit code: %w", err)
	}
	if code != 0 {
		return fmt.Errorf("elevated child exited with code %d", code)
	}
	return nil
}

// quoteArg wraps s in double quotes if it contains spaces or quotes, so
// the command line passed to ShellExecuteEx tokenizes back to exactly s.
func quoteArg(s string) string {
	if !strings.ContainsAny(s, " \t\"") {
		return s
	}
	return `"` + strings.ReplaceAll(s, `"`, `\"`) + `"`
}

// CleanupStaleJobs removes WriteJob temp files older than one hour that
// a previous crashed run may have left behind. Best-effort; errors are
// ignored. Safe to call on every startup of CLI or GUI.
func CleanupStaleJobs() {
	dir := os.TempDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	cutoff := time.Now().Add(-time.Hour)
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasPrefix(name, "ec2hosts-job-") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if info.ModTime().Before(cutoff) {
			_ = os.Remove(filepath.Join(dir, name))
		}
	}
}
