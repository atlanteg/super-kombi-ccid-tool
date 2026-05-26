package main

import (
	"encoding/csv"
	_ "embed"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
)

//go:embed cc_ids.csv
var embeddedCSV string

// CCIDEntry is a CC-ID with its description.
type CCIDEntry struct {
	ID          int
	Description string
}

// loadDescriptions returns a map of cc_id → description.
// First tries cc_ids.csv next to the executable; falls back to the embedded file.
func loadDescriptions() map[int]string {
	if data, err := readExternalCSV(); err == nil {
		return parseCSV(data)
	}
	return parseCSV(embeddedCSV)
}

func readExternalCSV() (string, error) {
	execPath, err := os.Executable()
	if err != nil {
		return "", err
	}
	// On macOS .app bundles the binary is inside Contents/MacOS/
	dir := filepath.Dir(execPath)
	if runtime.GOOS == "darwin" {
		// walk up to .app/../ if inside a bundle
		candidate := filepath.Join(dir, "..", "..", "..", "cc_ids.csv")
		if _, err := os.Stat(candidate); err == nil {
			b, err := os.ReadFile(candidate)
			return string(b), err
		}
	}
	b, err := os.ReadFile(filepath.Join(dir, "cc_ids.csv"))
	return string(b), err
}

func parseCSV(data string) map[int]string {
	r := csv.NewReader(strings.NewReader(data))
	r.Read() // skip header
	descs := make(map[int]string)
	for {
		record, err := r.Read()
		if err != nil {
			break
		}
		if len(record) < 2 {
			continue
		}
		id, err := strconv.Atoi(strings.TrimSpace(record[0]))
		if err != nil {
			continue
		}
		descs[id] = strings.TrimSpace(record[1])
	}
	return descs
}

// loadAllEntries returns all entries sorted by ID.
func loadAllEntries(descs map[int]string) []CCIDEntry {
	entries := make([]CCIDEntry, 0, len(descs))
	for id, desc := range descs {
		entries = append(entries, CCIDEntry{ID: id, Description: desc})
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].ID < entries[j].ID
	})
	return entries
}
