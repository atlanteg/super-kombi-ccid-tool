# BMW Kombi CC-ID Calculator

A cross-platform tool for calculating BMW instrument cluster (Kombi) CC-ID hex masks used in CAFD coding.

## Platforms

| Binary | Architecture | Runner |
|--------|-------------|--------|
| `bmw-ccid-calculator-win32.exe` | Windows 32-bit (i686) | Ubuntu + mingw-w64 |
| `bmw-ccid-calculator-macos-arm` | macOS Apple Silicon | macOS ARM |
| `bmw-ccid-calculator-macos-intel` | macOS Intel (x86-64) | macOS 13 |

Pre-built binaries are attached to each [GitHub Release](../../releases).

## How it works

BMW instrument clusters store CC-ID display permissions as bit masks in the CAFD coding file.  
Each group of **8 bytes = 64 CC-IDs**:

| Formula | Value |
|---------|-------|
| Group number | `cc_id // 64 + 1` |
| Position in group | `cc_id % 64` |
| Byte index (0–7) | `bit_pos // 8` |
| Bit index (0–7) | `bit_pos % 8` |
| Activate CC-ID | `byte[byte_idx] &= ~(1 << bit_idx)` |

**BMW convention:** `bit = 0` → CC-ID active (can appear), `bit = 1` → CC-ID masked.

To activate multiple CC-IDs in the same group the operations are applied sequentially to the same 8-byte array (no special summing needed — just repeated bit-clear).

## Workflow

1. **Step 1** — select CC-IDs you want to activate (search by number or description)
2. **Step 2** — enter the current hex bytes for each affected group (or load from a CAFD file)
3. **Step 3** — copy the modified hex values back into your coding tool

## Custom error database

The binary embeds `cc_ids.csv` at compile time. To use your own database without recompiling:

1. Create a `cc_ids.csv` file next to the executable:
   ```
   cc_id,description
   1,My custom error description
   2,Another error
   ```
2. The application loads this file automatically at startup.

## Build locally

Requires Go 1.21+ and a C compiler (Xcode CLI tools on macOS).

```bash
go build -o bmw-ccid-calculator .
```

## Build all platforms (GitHub Actions)

Push a tag to trigger the CI build:

```bash
git tag v1.0.0
git push origin v1.0.0
```

This builds all three platform binaries and creates a GitHub Release automatically.

## Algorithm reference

Algorithm reverse-engineered from `CCID-Calculator.exe` (PyInstaller/Python app) by disassembling the embedded `.pyc` bytecode with `pyinstxtractor` + Python 3.13 `dis`.
