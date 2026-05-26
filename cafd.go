package main

import (
	"bufio"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
)

var rePairs = regexp.MustCompile(`..`)

// parseCAFDFile parses a BMW CAFD file (Motorola S-record / SREC format).
// Extracts CC-ID byte groups from the function block at address 0x3001.
// Returns map[groupNum(1-based)] → 8-byte slice.
// Reproduced from _parse_cafd_file bytecode in CCID-Calculator.exe.
func parseCAFDFile(path string) (map[int][]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open: %w", err)
	}
	defer f.Close()

	// Collect hex payload from all S-records at address 3001.
	// S-record layout: S<type><LL><AAAA><data...><CC>
	//   [4:8]    = address (4 hex chars)
	//   [10:-2]  = data without checksum
	var sb strings.Builder
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "S") || len(line) <= 10 {
			continue
		}
		if line[4:8] != "3001" {
			continue
		}
		sb.WriteString(line[10 : len(line)-2])
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan: %w", err)
	}

	raw := sb.String()
	if raw == "" {
		return nil, nil
	}

	// Each group = 8 bytes = 16 hex chars.
	const chunkSize = 16
	results := make(map[int][]byte)
	for i := 0; i+chunkSize <= len(raw); i += chunkSize {
		chunk := raw[i : i+chunkSize]
		groupNum := i/chunkSize + 1

		pairs := rePairs.FindAllString(chunk, -1)
		groupBytes := make([]byte, 0, len(pairs))
		for _, pair := range pairs {
			val, err := strconv.ParseUint(pair, 16, 8)
			if err != nil {
				continue
			}
			groupBytes = append(groupBytes, byte(val))
		}
		results[groupNum] = groupBytes
	}
	return results, nil
}
