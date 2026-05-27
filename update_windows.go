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

// checkAndUpdate runs in a goroutine.
//
// Update flow:
//  1. Download new exe to %TEMP%\kombi-ccid-update.exe
//  2. Launch powershell.exe (trusted system process, not blocked by SmartScreen)
//     which waits 3 s, copies the download over the installed exe, then starts it.
//  3. os.Exit(0) — old process exits, releasing the file lock immediately.
//
// We intentionally do NOT exec the downloaded exe directly; running an
// unknown exe from %TEMP% is blocked silently by Windows Defender on many
// systems.  powershell.exe is always trusted.
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

	// Ask user.
	proceed := false
	mw.Synchronize(func() {
		res := walk.MsgBox(mw,
			"Update Available",
			fmt.Sprintf("Version %s is available (you have %s).\n\nDownload and install now?",
				rel.TagName, version),
			walk.MsgBoxYesNo|walk.MsgBoxIconInformation,
		)
		proceed = res == idYes
	})
	if !proceed {
		return
	}

	// Resolve where we are installed.
	exePath, err := os.Executable()
	if err != nil {
		showUpdateError(mw, "Cannot locate executable:\n"+err.Error())
		return
	}
	exePath, _ = filepath.EvalSymlinks(exePath)

	// Download into %TEMP% — always writable, no conflict with the running exe.
	tmpPath := filepath.Join(os.TempDir(), "kombi-ccid-update.exe")
	_ = os.Remove(tmpPath) // clear any leftover

	mw.Synchronize(func() {
		mw.SetTitle("BMW Kombi CC-ID Calculator — downloading update…")
	})

	if err := downloadFile(downloadURL, tmpPath); err != nil {
		mw.Synchronize(func() { mw.SetTitle("BMW Kombi CC-ID Calculator " + version) })
		_ = os.Remove(tmpPath)
		showUpdateError(mw, "Download failed:\n"+err.Error())
		return
	}

	// Hand off to PowerShell — a trusted system process that won't be blocked
	// by SmartScreen or Defender.  It waits for our process to release the file
	// lock, copies the download over the installed exe, then starts the new ver.
	//
	// Single-quoted PS strings: literal; a single-quote inside is doubled ('').
	psq := func(s string) string {
		return "'" + strings.ReplaceAll(s, "'", "''") + "'"
	}
	psCmd := fmt.Sprintf(
		`Start-Sleep -Seconds 3; `+
			`try { `+
			`  Copy-Item -LiteralPath %s -Destination %s -Force; `+
			`  Start-Process -FilePath %s `+
			`} catch { `+
			`  Start-Process -FilePath %s `+
			`}`,
		psq(tmpPath), psq(exePath), psq(exePath), psq(tmpPath),
	)

	cmd := exec.Command("powershell.exe",
		"-WindowStyle", "Hidden",
		"-NonInteractive",
		"-Command", psCmd,
	)
	if err := cmd.Start(); err != nil {
		_ = os.Remove(tmpPath)
		mw.Synchronize(func() { mw.SetTitle("BMW Kombi CC-ID Calculator " + version) })
		showUpdateError(mw, "Cannot launch updater:\n"+err.Error())
		return
	}

	// Exit — file lock on exePath released immediately.  PowerShell takes over.
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

func showUpdateError(mw *walk.MainWindow, msg string) {
	mw.Synchronize(func() {
		walk.MsgBox(mw, "Update Error", msg, walk.MsgBoxIconError)
	})
}
