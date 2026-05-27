//go:build windows

package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/lxn/walk"
)

const (
	ghReleasesAPI = "https://api.github.com/repos/atlanteg/super-kombi-ccid-tool/releases/latest"
	updateAsset   = "kombi-ccid-win32.exe"
	idYes         = 6 // Windows IDYES
)

type ghRelease struct {
	TagName string    `json:"tag_name"`
	Assets  []ghAsset `json:"assets"`
}

type ghAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

// checkAndUpdate runs in a goroutine on startup.
//
// Update flow (fully automatic, no user confirmation):
//  1. Fetch latest release from GitHub API.
//  2. If newer — download new exe to %TEMP%\kombi-ccid-update.exe.
//  3. Launch cmd.exe as a detached hidden process with CREATE_NEW_PROCESS_GROUP
//     so it survives our os.Exit(0) even if we are inside a Windows Job Object.
//     The cmd command: wait 3 s (via ping), copy new exe over current one, start it.
//  4. os.Exit(0) — releases the file lock on the current exe immediately.
//
// Why cmd.exe instead of PowerShell or the downloaded exe:
//   - Running an unknown exe from %TEMP% is silently killed by Defender on most
//     systems.
//   - PowerShell's child process can be killed together with the parent if both are
//     in the same Windows Job Object (common in some launcher contexts).
//   - cmd.exe launched with CREATE_NEW_PROCESS_GROUP breaks the Job Object
//     inheritance and survives parent exit reliably.
//   - Since the app requires administrator (manifest), cmd.exe inherits the token
//     and can write to any location.
func checkAndUpdate(mw *walk.MainWindow) {
	if version == "dev" {
		return
	}

	rel, err := fetchRelease()
	if err != nil || rel == nil {
		return
	}
	if !versionNewer(rel.TagName, version) {
		return
	}

	var downloadURL string
	for _, a := range rel.Assets {
		if a.Name == updateAsset {
			downloadURL = a.BrowserDownloadURL
			break
		}
	}
	if downloadURL == "" {
		return
	}

	// Resolve the path of the currently running executable.
	exePath, err := os.Executable()
	if err != nil {
		return
	}
	exePath, _ = filepath.EvalSymlinks(exePath)

	// Download into %TEMP% — always writable, no conflict with the running exe.
	tmpPath := filepath.Join(os.TempDir(), "kombi-ccid-update.exe")
	_ = os.Remove(tmpPath)

	mw.Synchronize(func() {
		mw.SetTitle(fmt.Sprintf("BMW Kombi CC-ID Calculator %s — updating to %s…", version, rel.TagName))
	})

	if err := downloadFile(downloadURL, tmpPath); err != nil {
		mw.Synchronize(func() {
			mw.SetTitle("BMW Kombi CC-ID Calculator " + version)
		})
		return // silent — no error dialog for auto-update
	}

	// Build the cmd.exe command:
	//   ping 127.0.0.1 -n 4   → ~3 second delay (4 ICMP replies, 1 s apart)
	//   copy /y src dst        → overwrite the installed exe
	//   start "" dst           → launch the newly installed version
	//
	// Paths are double-quoted; double-quotes are invalid in Windows paths so no
	// extra escaping is needed.
	installCmd := fmt.Sprintf(
		`ping 127.0.0.1 -n 4 >NUL & copy /y "%s" "%s" & start "" "%s"`,
		tmpPath, exePath, exePath,
	)
	cmd := exec.Command("cmd.exe", "/c", installCmd)
	// CREATE_NEW_PROCESS_GROUP (0x200) + CREATE_NO_WINDOW (0x8000000):
	// - breaks Job Object inheritance → cmd.exe survives our os.Exit(0)
	// - no console window is shown
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: 0x00000200 | 0x08000000, // CREATE_NEW_PROCESS_GROUP | CREATE_NO_WINDOW
	}

	if err := cmd.Start(); err != nil {
		mw.Synchronize(func() {
			mw.SetTitle("BMW Kombi CC-ID Calculator " + version)
		})
		return
	}

	// Exit immediately — releases the file lock so cmd.exe can overwrite the exe.
	os.Exit(0)
}

// ── helpers ───────────────────────────────────────────────────────────────────

func fetchRelease() (*ghRelease, error) {
	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequest("GET", ghReleasesAPI, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "super-kombi-ccid-tool/"+version)

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API HTTP %d", resp.StatusCode)
	}

	var rel ghRelease
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return nil, err
	}
	return &rel, nil
}

func downloadFile(url, dest string) error {
	client := &http.Client{Timeout: 10 * time.Minute}
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("server HTTP %d", resp.StatusCode)
	}
	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, resp.Body)
	return err
}

func versionNewer(latest, current string) bool {
	l, c := parseVer(latest), parseVer(current)
	for i := range l {
		if l[i] > c[i] {
			return true
		}
		if l[i] < c[i] {
			return false
		}
	}
	return false
}

func parseVer(v string) [3]int {
	v = strings.TrimPrefix(v, "v")
	parts := strings.SplitN(v, ".", 3)
	var r [3]int
	for i, p := range parts {
		if i >= 3 {
			break
		}
		if idx := strings.IndexAny(p, "-+"); idx >= 0 {
			p = p[:idx]
		}
		r[i], _ = strconv.Atoi(p)
	}
	return r
}
