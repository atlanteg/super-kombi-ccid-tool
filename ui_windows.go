//go:build windows

package main

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/lxn/walk"
	. "github.com/lxn/walk/declarative"
)

// "Read from Car" section constants
const defaultVCIHost = "169.254.138.176"

type winApp struct {
	mw          *walk.MainWindow
	lbAvailable *walk.ListBox
	lbSelected  *walk.ListBox
	leSearch    *walk.LineEdit
	lblStatus   *walk.Label
	teHex       *walk.TextEdit
	teResults   *walk.TextEdit

	// "Read from Car" section
	leVCIHost  *walk.LineEdit
	teLiveRes  *walk.TextEdit
	lblLive    *walk.Label

	allEntries  []CCIDEntry
	filtered    []CCIDEntry
	selectedIDs map[int]bool
}

func run() {
	all := loadAllEntries()
	wa := &winApp{
		allEntries:  all,
		filtered:    all,
		selectedIDs: make(map[int]bool),
	}

	if err := (MainWindow{
		AssignTo: &wa.mw,
		Title:    "BMW Kombi CC-ID Calculator",
		Size:     Size{Width: 920, Height: 820},
		Layout:   VBox{MarginsZero: true},
		Children: []Widget{
			Composite{
				Layout: VBox{Margins: Margins{Left: 8, Top: 8, Right: 8, Bottom: 8}},
				Children: []Widget{

					// ── Step 1: Select CC-IDs ─────────────────────────────────
					GroupBox{
						Title:  "Step 1 — Select CC-IDs (double-click to add / remove)",
						Layout: VBox{},
						Children: []Widget{

							Composite{
								Layout: HBox{},
								Children: []Widget{
									Label{Text: "Search:"},
									LineEdit{
										AssignTo: &wa.leSearch,
										OnTextChanged: func() {
											wa.applyFilter()
										},
									},
									Label{
										AssignTo: &wa.lblStatus,
										Text:     "0 selected",
									},
								},
							},

							HSplitter{
								Children: []Widget{
									Composite{
										Layout: VBox{MarginsZero: true},
										Children: []Widget{
											Label{Text: "Available (double-click → add):"},
											ListBox{
												AssignTo:        &wa.lbAvailable,
												OnItemActivated: func() { wa.addSelected() },
											},
										},
									},
									Composite{
										Layout: VBox{MarginsZero: true},
										Children: []Widget{
											Label{Text: "Selected (double-click → remove):"},
											ListBox{
												AssignTo:        &wa.lbSelected,
												OnItemActivated: func() { wa.removeSelected() },
											},
										},
									},
								},
							},

							Composite{
								Layout: HBox{},
								Children: []Widget{
									PushButton{Text: ">> Add", OnClicked: func() { wa.addSelected() }},
									PushButton{Text: "<< Remove", OnClicked: func() { wa.removeSelected() }},
									PushButton{Text: "Clear All", OnClicked: func() {
										wa.selectedIDs = make(map[int]bool)
										wa.refreshLists()
										wa.refreshHexTemplate()
									}},
								},
							},
						},
					},

					// ── Step 2: Hex input ─────────────────────────────────────
					GroupBox{
						Title:  "Step 2 — Current Hex Values (from CAFD; default FF = all masked)",
						Layout: VBox{},
						Children: []Widget{
							Label{
								Text: "One data line per group:  GROUP_N: XX XX XX XX XX XX XX XX\n" +
									"Lines starting with # are comments and are ignored by the parser.",
							},
							Composite{
								Layout: HBox{},
								Children: []Widget{
									PushButton{
										Text:      "Load from CAFD file…",
										OnClicked: func() { wa.loadCAFD() },
									},
									PushButton{
										Text:      "Reset all to FF",
										OnClicked: func() { wa.refreshHexTemplate() },
									},
								},
							},
							TextEdit{
								AssignTo: &wa.teHex,
								MinSize:  Size{Height: 130},
							},
						},
					},

					// ── Calculate button ──────────────────────────────────────
					PushButton{
						Text:      "CALCULATE",
						OnClicked: func() { wa.calculate() },
					},

					// ── Results ───────────────────────────────────────────────
					GroupBox{
						Title:  "Results",
						Layout: VBox{},
						Children: []Widget{
							PushButton{
								Text:      "Copy to Clipboard",
								OnClicked: func() { wa.copyResults() },
							},
							TextEdit{
								AssignTo: &wa.teResults,
								ReadOnly: true,
								MinSize:  Size{Height: 130},
							},
						},
					},

					// ── Read from Car ─────────────────────────────────────────
					GroupBox{
						Title:  "Read CC-IDs from Connected Car (EDIABAS/TCP)",
						Layout: VBox{},
						Children: []Widget{
							Label{
								Text: "Enter the IP address of your BMW VCI adapter (link-local, port 6801).\n" +
									"The car must be ignition-on and the OBD cable plugged in.",
							},
							Composite{
								Layout: HBox{},
								Children: []Widget{
									Label{Text: "VCI IP:"},
									LineEdit{
										AssignTo: &wa.leVCIHost,
										Text:     defaultVCIHost,
										MinSize:  Size{Width: 140},
									},
									PushButton{
										Text:      "Read from Car",
										OnClicked: func() { wa.readFromCar() },
									},
									Label{
										AssignTo: &wa.lblLive,
										Text:     "",
									},
								},
							},
							TextEdit{
								AssignTo: &wa.teLiveRes,
								ReadOnly: true,
								MinSize:  Size{Height: 150},
							},
						},
					},
				},
			},
		},
	}).Create(); err != nil {
		panic(err)
	}

	wa.applyFilter()
	wa.mw.Run()
}

// ── CC-ID list helpers ────────────────────────────────────────────────────────

func (a *winApp) applyFilter() {
	q := strings.ToLower(strings.TrimSpace(a.leSearch.Text()))
	if q == "" {
		a.filtered = a.allEntries
	} else {
		var f []CCIDEntry
		for _, e := range a.allEntries {
			if matchesQuery(e, q) {
				f = append(f, e)
			}
		}
		a.filtered = f
	}
	a.refreshLists()
}

func (a *winApp) refreshLists() {
	items := make([]string, len(a.filtered))
	for i, e := range a.filtered {
		items[i] = fmt.Sprintf("%-5d  %s", e.ID, e.Description)
	}
	a.lbAvailable.SetModel(items)

	sel := a.getSelected()
	selItems := make([]string, len(sel))
	for i, e := range sel {
		selItems[i] = fmt.Sprintf("%-5d  %s", e.ID, e.Description)
	}
	a.lbSelected.SetModel(selItems)
	a.lblStatus.SetText(fmt.Sprintf("%d selected", len(a.selectedIDs)))
}

func (a *winApp) addSelected() {
	idx := a.lbAvailable.CurrentIndex()
	if idx < 0 || idx >= len(a.filtered) {
		return
	}
	a.selectedIDs[a.filtered[idx].ID] = true
	a.refreshLists()
	a.refreshHexTemplate()
}

func (a *winApp) removeSelected() {
	idx := a.lbSelected.CurrentIndex()
	if idx < 0 {
		return
	}
	sel := a.getSelected()
	if idx >= len(sel) {
		return
	}
	delete(a.selectedIDs, sel[idx].ID)
	a.refreshLists()
	a.refreshHexTemplate()
}

func (a *winApp) getSelected() []CCIDEntry {
	entries := make([]CCIDEntry, 0, len(a.selectedIDs))
	for id := range a.selectedIDs {
		var desc string
		for _, ae := range a.allEntries {
			if ae.ID == id {
				desc = ae.Description
				break
			}
		}
		entries = append(entries, CCIDEntry{ID: id, Description: desc})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].ID < entries[j].ID })
	return entries
}

// ── Hex template ──────────────────────────────────────────────────────────────

// refreshHexTemplate regenerates the hex input area for the affected groups,
// preserving any values the user already typed.
func (a *winApp) refreshHexTemplate() {
	existing := a.parseHexText()
	groups := a.affectedGroups()
	if len(groups) == 0 {
		a.teHex.SetText("")
		return
	}

	var sb strings.Builder
	for _, gn := range groups {
		minID := (gn - 1) * 64
		maxID := gn*64 - 1
		var ids []int
		for id := range a.selectedIDs {
			if id >= minID && id <= maxID {
				ids = append(ids, id)
			}
		}
		sort.Ints(ids)
		idStrs := make([]string, len(ids))
		for i, id := range ids {
			idStrs[i] = strconv.Itoa(id)
		}
		sb.WriteString(fmt.Sprintf("# Group %d (CC-IDs %d-%d)  activating: %s\n",
			gn, minID, maxID, strings.Join(idStrs, ", ")))

		var b [8]byte
		for i := range b {
			b[i] = 0xFF
		}
		if ex, ok := existing[gn]; ok && len(ex) == 8 {
			copy(b[:], ex)
		}
		hexParts := make([]string, 8)
		for i, v := range b {
			hexParts[i] = fmt.Sprintf("%02X", v)
		}
		sb.WriteString(fmt.Sprintf("GROUP_%d: %s\n\n", gn, strings.Join(hexParts, " ")))
	}
	a.teHex.SetText(strings.TrimRight(sb.String(), "\n"))
}

// parseHexText reads GROUP_N: XX XX XX XX XX XX XX XX lines from the TextEdit.
func (a *winApp) parseHexText() map[int][]byte {
	result := make(map[int][]byte)
	for _, line := range strings.Split(a.teHex.Text(), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if !strings.HasPrefix(line, "GROUP_") {
			continue
		}
		rest := line[6:]
		colon := strings.Index(rest, ":")
		if colon < 0 {
			continue
		}
		gn, err := strconv.Atoi(strings.TrimSpace(rest[:colon]))
		if err != nil {
			continue
		}
		parts := strings.Fields(strings.TrimSpace(rest[colon+1:]))
		if len(parts) != 8 {
			continue
		}
		b := make([]byte, 8)
		ok := true
		for i, p := range parts {
			v, err := strconv.ParseUint(strings.ToUpper(p), 16, 8)
			if err != nil {
				ok = false
				break
			}
			b[i] = byte(v)
		}
		if ok {
			result[gn] = b
		}
	}
	return result
}

func (a *winApp) affectedGroups() []int {
	seen := make(map[int]bool)
	for id := range a.selectedIDs {
		seen[getGroupNumber(id)] = true
	}
	groups := make([]int, 0, len(seen))
	for g := range seen {
		groups = append(groups, g)
	}
	sort.Ints(groups)
	return groups
}

// ── CAFD loader ───────────────────────────────────────────────────────────────

func (a *winApp) loadCAFD() {
	dlg := new(walk.FileDialog)
	dlg.Title = "Load CAFD / S-record file"
	dlg.Filter = "CAFD/S-record (*.cafd;*.sre;*.s19;*.srec;*.txt)|*.cafd;*.sre;*.s19;*.srec;*.txt|All files (*.*)|*.*"
	if ok, err := dlg.ShowOpen(a.mw); err != nil || !ok {
		return
	}

	cafdData, err := parseCAFDFile(dlg.FilePath)
	if err != nil {
		walk.MsgBox(a.mw, "Error", "Cannot parse CAFD:\n"+err.Error(), walk.MsgBoxIconError)
		return
	}
	if cafdData == nil {
		walk.MsgBox(a.mw, "Not found", "No CC-ID block (address 3001) found in this file.", walk.MsgBoxIconWarning)
		return
	}

	existing := a.parseHexText()
	for gn, b := range cafdData {
		existing[gn] = b
	}

	groups := a.affectedGroups()
	var sb strings.Builder
	for _, gn := range groups {
		minID := (gn - 1) * 64
		maxID := gn*64 - 1
		var ids []int
		for id := range a.selectedIDs {
			if id >= minID && id <= maxID {
				ids = append(ids, id)
			}
		}
		sort.Ints(ids)
		idStrs := make([]string, len(ids))
		for i, id := range ids {
			idStrs[i] = strconv.Itoa(id)
		}
		var b [8]byte
		for i := range b {
			b[i] = 0xFF
		}
		if ex, ok := existing[gn]; ok && len(ex) == 8 {
			copy(b[:], ex)
		}
		hexParts := make([]string, 8)
		for i, v := range b {
			hexParts[i] = fmt.Sprintf("%02X", v)
		}
		sb.WriteString(fmt.Sprintf("# Group %d (CC-IDs %d-%d)  activating: %s\n",
			gn, minID, maxID, strings.Join(idStrs, ", ")))
		sb.WriteString(fmt.Sprintf("GROUP_%d: %s\n\n", gn, strings.Join(hexParts, " ")))
	}
	a.teHex.SetText(strings.TrimRight(sb.String(), "\n"))
}

// ── Calculate ─────────────────────────────────────────────────────────────────

func (a *winApp) calculate() {
	if len(a.selectedIDs) == 0 {
		walk.MsgBox(a.mw, "Nothing selected", "Please select at least one CC-ID first.", walk.MsgBoxIconWarning)
		return
	}
	initialStates := a.parseHexText()
	for _, gn := range a.affectedGroups() {
		if _, ok := initialStates[gn]; !ok {
			b := make([]byte, 8)
			for i := range b {
				b[i] = 0xFF
			}
			initialStates[gn] = b
		}
	}
	ids := make([]int, 0, len(a.selectedIDs))
	for id := range a.selectedIDs {
		ids = append(ids, id)
	}

	results := calculateMask(initialStates, ids)
	var sb strings.Builder
	for _, gr := range results {
		sb.WriteString(fmt.Sprintf("Group %d (CC-IDs %d-%d)\n",
			gr.GroupNum, (gr.GroupNum-1)*64, gr.GroupNum*64-1))
		sb.WriteString("  Before: " + bytesToHex(gr.OriginalBytes) + "\n")
		sb.WriteString("  After:  " + bytesToHex(gr.ModifiedBytes) + "\n")
		for _, idx := range gr.ModifiedIndices {
			sb.WriteString(fmt.Sprintf("  Byte %d: %02X -> %02X\n",
				idx+1, gr.OriginalBytes[idx], gr.ModifiedBytes[idx]))
		}
		sb.WriteString("\n")
	}
	a.teResults.SetText(strings.TrimRight(sb.String(), "\n"))
}

func (a *winApp) copyResults() {
	text := a.teResults.Text()
	if text == "" {
		walk.MsgBox(a.mw, "Nothing to copy", "Run Calculate first.", walk.MsgBoxIconInformation)
		return
	}
	walk.Clipboard().SetText(text)
}

// ── Read from Car ─────────────────────────────────────────────────────────────

func (a *winApp) readFromCar() {
	host := strings.TrimSpace(a.leVCIHost.Text())
	if host == "" {
		walk.MsgBox(a.mw, "No IP", "Enter the VCI IP address.", walk.MsgBoxIconWarning)
		return
	}
	a.lblLive.SetText("Connecting…")
	a.teLiveRes.SetText("")

	go func() {
		ccids, err := ReadVehicleCCIDs(host)

		a.mw.Synchronize(func() {
			if err != nil {
				a.lblLive.SetText("Error")
				walk.MsgBox(a.mw, "Connection Error", err.Error(), walk.MsgBoxIconError)
				return
			}
			if len(ccids) == 0 {
				a.lblLive.SetText("0 CC-IDs found")
				a.teLiveRes.SetText("No active CC-IDs reported by the cluster.")
				return
			}

			a.lblLive.SetText(fmt.Sprintf("%d CC-IDs found", len(ccids)))
			var sb strings.Builder
			sb.WriteString(fmt.Sprintf("Found %d stored CC-IDs:\n\n", len(ccids)))
			for _, c := range ccids {
				sb.WriteString(fmt.Sprintf("CC-ID %4d  %s\n", c.ID, c.Description))
			}
			a.teLiveRes.SetText(strings.TrimRight(sb.String(), "\n"))
		})
	}()
}

// ── shared helper (also in ui_darwin.go) ─────────────────────────────────────

func bytesToHex(b []byte) string {
	parts := make([]string, len(b))
	for i, v := range b {
		parts[i] = fmt.Sprintf("%02X", v)
	}
	return strings.Join(parts, " ")
}
