//go:build windows

package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"
)

func main() {
	// When launched as the installer helper by an older auto-updater:
	//   kombi-ccid-win32.exe --install-to <targetPath>
	//
	// The old process has already exited, so targetPath is unlocked.
	// We copy ourselves there and start the installed copy — no UI shown.
	if len(os.Args) >= 3 && os.Args[1] == "--install-to" {
		selfInstall(os.Args[2])
		return
	}
	run()
}

// selfInstall runs from the downloaded temp location (invoked by older app versions
// that still use the --install-to mechanism). It waits for the target file to become
// writable (old process released its lock), copies itself there, then starts the
// installed version.
func selfInstall(targetPath string) {
	logPath := filepath.Join(os.TempDir(), "kombi-ccid-install.log")
	logf := func(format string, args ...any) {
		f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return
		}
		fmt.Fprintf(f, time.Now().Format("15:04:05")+" "+format+"\n", args...)
		f.Close()
	}

	self, err := os.Executable()
	if err != nil {
		logf("os.Executable error: %v", err)
		os.Exit(1)
	}
	logf("selfInstall started: self=%s target=%s", self, targetPath)

	// Poll until targetPath is writable (old process released the file lock).
	const (
		pollInterval = 200 * time.Millisecond
		pollTimeout  = 15 * time.Second
	)
	deadline := time.Now().Add(pollTimeout)
	for {
		f, err := os.OpenFile(targetPath, os.O_WRONLY, 0)
		if err == nil {
			f.Close()
			break
		}
		if time.Now().After(deadline) {
			logf("timeout waiting for file lock on %s", targetPath)
			break
		}
		time.Sleep(pollInterval)
	}
	logf("file writable, copying…")

	if err := fileCopy(self, targetPath); err != nil {
		logf("fileCopy error: %v — running from temp as fallback", err)
		startDetached(self)
		os.Exit(0)
	}

	logf("copy succeeded, starting %s", targetPath)
	startDetached(targetPath)
	os.Exit(0)
}

// startDetached launches exe as a fully independent process (CREATE_NEW_PROCESS_GROUP)
// so it is not affected by the current process's Job Object or lifetime.
func startDetached(exe string) {
	cmd := exec.Command(exe)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: 0x00000200, // CREATE_NEW_PROCESS_GROUP
	}
	if err := cmd.Start(); err != nil {
		// Last resort: plain Start without special flags.
		exec.Command(exe).Start() //nolint
	}
}

func fileCopy(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}
