# BMW Kombi CC-ID Calculator

Cross-platform tool for calculating BMW instrument cluster (Kombi) CC-ID hex masks used in CAFD coding.

## Platforms

| Binary | UI framework | Architecture | Min OS |
|--------|-------------|--------------|--------|
| `bmw-ccid-calculator-win32.exe` | Native Win32 (`lxn/walk`) | x86 32-bit | Windows 7 |
| `bmw-ccid-calculator-macos-arm` | Fyne | Apple Silicon (arm64) | macOS 11 |
| `bmw-ccid-calculator-macos-intel` | Fyne | Intel (amd64) | macOS 10.15 |

Pre-built binaries are attached to each [GitHub Release](../../releases).

## How it works

BMW instrument clusters store CC-ID display permissions as bit masks in the CAFD file.  
Each group of **8 bytes = 64 CC-IDs**:

| Formula | Value |
|---------|-------|
| Group number | `cc_id // 64 + 1` |
| Position in group | `cc_id % 64` |
| Byte index (0–7) | `bit_pos // 8` |
| Bit index (0–7) | `bit_pos % 8` |
| Activate CC-ID | `byte[byte_idx] &= ~(1 << bit_idx)` |

**BMW convention:** `bit = 0` → CC-ID active (can appear), `bit = 1` → CC-ID masked/suppressed.

Multiple CC-IDs in the same group are combined by applying the bit-clear operation sequentially to the same 8-byte array.

## Workflow — Windows (native Win32)

Single-page interface with three sections visible simultaneously:

1. **Step 1** — search and select CC-IDs (double-click to add/remove)
2. **Step 2** — edit hex values for each affected group (or load from CAFD file)
3. **CALCULATE** — click the button; results appear in the bottom panel

Hex input format in the text area:
```
# Group 1 (CC-IDs 0-63)  activating: 27
GROUP_1: FF FF FF FF FF FF FF FF

# Group 2 (CC-IDs 64-127)  activating: 63, 71
GROUP_2: FF FF FF FF FF FF FF FF
```

## Workflow — macOS (Fyne, 3-step wizard)

1. **Step 1** — search and select CC-IDs  
2. **Step 2** — enter current hex bytes (or load from CAFD file)  
3. **Step 3** — copy the modified hex values

## Custom error database

The binary embeds `cc_ids.csv` at compile time.  
To use your own database **without recompiling**:

1. Place a `cc_ids.csv` file next to the executable.
2. The application will load it automatically at startup.

CSV format (required columns):
```
CC_ID,TITLE_ENGB,LONGTEXT_ENGB,...
1,Cruise Control disabled,...
```

The parser accepts the full multi-language BMW format (with `TITLE_ENUS`, `TITLE_DEDE`, etc.) and automatically falls back to `TITLE_ENUS` when `TITLE_ENGB` is missing.

## Read CC-IDs from a Connected Car

The tool can query the instrument cluster directly over the EDIABAS/TCP interface exposed by BMW VCI adapters (ISTA-compatible dongles).

**Requirements:**
- BMW VCI adapter connected via OBD-II (e.g. ICOM, ENET cable with ISTA)
- Car ignition ON
- VCI reachable on the link-local network (typically `169.254.x.x`, port **6801**)

**Windows:** enter the VCI IP in the "Read CC-IDs from Connected Car" section and click **Read from Car**.

**macOS:** click **🔌 Read from Car…** on Step 1, enter the VCI IP, press **Connect & Read**.

The app sends a UDS `ReadDataByIdentifier` (service `0x22`, DID `0xD10B`) to the Kombi ECU (address `0x60`) wrapped in the EDIABAS/AIFC TCP framing observed in traffic captures, then looks up each returned CC-ID in the embedded database and displays it with its description.

**Protocol (for reference):**
```
Frame:   [4B length (BE)][2B type: 0001=data / 0002=ack][payload]
Request payload:  F4 60 22 D1 0B
Response payload: 60 F4 62 D1 0B [4-byte records...]
Record:  [CC_ID_HI][CC_ID_LO][FLAG1][FLAG2] — zero record = end of list
```

## Build

**macOS (native, current machine):**
```bash
go build -o bmw-ccid-calculator .
```

**Windows 32-bit (cross-compile from macOS/Linux):**
```bash
# macOS: brew install mingw-w64
# Linux: sudo apt-get install gcc-mingw-w64-i686
GOOS=windows GOARCH=386 CGO_ENABLED=1 CC=i686-w64-mingw32-gcc \
  go build -ldflags "-H windowsgui -s -w" -o bmw-ccid-calculator-win32.exe .
```

**Trigger CI builds for all 3 platforms:**
```bash
git tag v1.0.0
git push origin v1.0.0
```

## Go version

Requires **Go 1.20** (last version with Windows 7 support).  
Algorithm reverse-engineered from `CCID-Calculator.exe` (PyInstaller/Python) by disassembling the embedded `.pyc` bytecode.
