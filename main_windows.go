//go:build windows

package main

import (
	"io"
	"os"
	"os/exec"
	"time"
)

func main() {
	// When launched as the installer helper by the auto-updater:
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

// selfInstall runs from the downloaded temp location, waits for the old
// process to release its file lock, copies itself to targetPath, starts it.
func selfInstall(targetPath string) {
	// Wait for the old process to fully exit.
	time.Sleep(2 * time.Second)

	self, err := os.Executable()
	if err != nil {
		os.Exit(1)
	}

	if err := fileCopy(self, targetPath); err != nil {
		// Permission error or AV blocking — run from temp as fallback.
		exec.Command(self).Start() //nolint
		os.Exit(0)
	}

	// Launch the freshly installed version and disappear.
	exec.Command(targetPath).Start() //nolint
	os.Exit(0)
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
