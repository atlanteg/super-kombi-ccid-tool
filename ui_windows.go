//go:build windows

package main

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/atlanteg/bmw-kombi-ccid-tool/bmwzgw"
	"github.com/lxn/walk"
	. "github.com/lxn/walk/declarative"
)

const defaultVCIHost = "169.254.138.176"

type winApp struct {
	mw *walk.MainWindow
	tw *walk.TabWidget

	// Tab 2 — Select CC-IDs
	lbAvailable *walk.ListBox
	lbSelected  *walk.ListBox
	leSearch    *walk.LineEdit
	lblStatus   *walk.Label

	// Tab 3 — Calculate Mask
	hexComp   *walk.Composite       // container for per-group hex input rows
	hexFields map[int]*walk.LineEdit // group# → LineEdit
	resComp   *walk.Composite       // container for per-group result rows

	// Tab 1 — Read from Car
	cbAutoSearch *walk.CheckBox
	leZGW        *walk.LineEdit  // auto-search ZGW discovery status
	leVCIHost    *walk.LineEdit
	lblLive      *walk.LineEdit  // connection result / error (separate row)
	lbLive       *walk.ListBox
	liveCCIDs    []LiveCCID
	zgwCancel    context.CancelFunc

	allEntries  []CCIDEntry
	filtered    []CCIDEntry
	selectedIDs map[int]bool
	descMode        int            // 0=EnUS 1=EnUS_Long 2=DeDe 3=DeDe_Long 4=EnGB 5=EnGB_Long
	cbDescMode      *walk.ComboBox // Tab 2 language selector
	cbDescMode2     *walk.ComboBox // Tab 1 language selector (mirrors cbDescMode)
	syncingDescMode bool           // re-entrancy guard for setDescMode
}

func run() {
	all := loadAllEntries()
	wa := &winApp{
		allEntries:  all,
		filtered:    all,
		selectedIDs: make(map[int]bool),
		descMode:    4, // EnGB — matches previous default
		hexFields:   make(map[int]*walk.LineEdit),
	}

	title := "BMW Kombi CC-ID Calculator"
	if version != "dev" {
		title += " " + version
	}

	if err := (MainWindow{
		AssignTo: &wa.mw,
		Title:    title,
		MinSize:  Size{Width: 780, Height: 560},
		Size:     Size{Width: 980, Height: 720},
		Layout:   VBox{MarginsZero: true},
		Children: []Widget{

			TabWidget{
				AssignTo: &wa.tw,
				Pages: []TabPage{

					// ══════════════════════════════════════════════════════
					// Tab 1 — Read from Car
					// ══════════════════════════════════════════════════════
					{
						Title:  "  Read from Car  ",
						Layout: VBox{Margins: Margins{Left: 8, Top: 8, Right: 8, Bottom: 8}},
						Children: []Widget{

							Label{Text: "Reads stored CC-IDs from the instrument cluster via EDIABAS/TCP (port 6801)."},
							Label{Text: "Requirements: BMW VCI adapter connected via OBD-II cable, ignition ON."},

							// Auto-search row: checkbox + ZGW discovery status
							Composite{
								Layout: HBox{MarginsZero: true},
								Children: []Widget{
									CheckBox{
										AssignTo: &wa.cbAutoSearch,
										Text:     "Auto-search",
										Checked:  true,
										OnCheckedChanged: func() {
											if wa.cbAutoSearch.Checked() {
												wa.leVCIHost.SetReadOnly(true)
												wa.startAutoSearch()
											} else {
												wa.stopAutoSearch()
												wa.leVCIHost.SetReadOnly(false)
												wa.leZGW.SetText("Auto-search disabled — enter IP manually")
											}
										},
									},
									LineEdit{
										AssignTo: &wa.leZGW,
										ReadOnly: true,
										Text:     "",
									},
								},
							},

							// IP + connect row (no inline status label)
							Composite{
								Layout: HBox{MarginsZero: true},
								Children: []Widget{
									Label{Text: "VCI IP address:"},
									LineEdit{
										AssignTo: &wa.leVCIHost,
										Text:     defaultVCIHost,
										MaxSize:  Size{Width: 160},
									},
									PushButton{
										Text:      "Read from Car",
										OnClicked: func() { wa.readFromCar() },
									},
								},
							},

							// Connection result / error — own row, full width, never breaks layout
							LineEdit{
								AssignTo: &wa.lblLive,
								ReadOnly: true,
								Text:     "",
							},

							// Live CC-ID list
							Label{Text: "CC-IDs stored in cluster  (double-click → add to Selected)"},
							ListBox{
								AssignTo:        &wa.lbLive,
								OnItemActivated: func() { wa.addFromCar() },
							},

							// Bottom buttons
							Composite{
								Layout: HBox{MarginsZero: true},
								Children: []Widget{
									PushButton{
										Text:      "Add All to Selected",
										OnClicked: func() { wa.addAllFromCar() },
									},
									HSpacer{},
									Label{Text: "View:"},
									ComboBox{
										AssignTo: &wa.cbDescMode2,
										Model:    descModeNames,
										Value:    descModeNames[4],
										MaxSize:  Size{Width: 140},
										OnCurrentIndexChanged: func() {
											wa.setDescMode(wa.cbDescMode2.CurrentIndex())
										},
									},
									HSpacer{},
									PushButton{
										Text: "→ Go to Select CC-IDs tab",
										OnClicked: func() {
											wa.tw.SetCurrentIndex(1)
										},
									},
								},
							},
						},
					},

					// ══════════════════════════════════════════════════════
					// Tab 2 — Select CC-IDs
					// ══════════════════════════════════════════════════════
					{
						Title:  "  Select CC-IDs  ",
						Layout: VBox{Margins: Margins{Left: 8, Top: 8, Right: 8, Bottom: 8}},
						Children: []Widget{

							// Search + view selector + status
							Composite{
								Layout: HBox{MarginsZero: true},
								Children: []Widget{
									Label{Text: "Search:"},
									LineEdit{
										AssignTo:      &wa.leSearch,
										OnTextChanged: func() { wa.applyFilter() },
									},
									Label{Text: "  View:"},
									ComboBox{
										AssignTo: &wa.cbDescMode,
										Model:    descModeNames,
										Value:    descModeNames[4],
										MaxSize:  Size{Width: 140},
										OnCurrentIndexChanged: func() {
											wa.setDescMode(wa.cbDescMode.CurrentIndex())
										},
									},
									Label{AssignTo: &wa.lblStatus, Text: "0 selected"},
								},
							},

							// Available | Selected
							HSplitter{
								Children: []Widget{
									Composite{
										Layout: VBox{MarginsZero: true},
										Children: []Widget{
											Label{Text: "Available  (double-click → add)"},
											ListBox{
												AssignTo:        &wa.lbAvailable,
												OnItemActivated: func() { wa.addSelected() },
											},
										},
									},
									Composite{
										Layout: VBox{MarginsZero: true},
										Children: []Widget{
											Label{Text: "Selected  (double-click → remove)"},
											ListBox{
												AssignTo:        &wa.lbSelected,
												OnItemActivated: func() { wa.removeSelected() },
											},
										},
									},
								},
							},

							// Action buttons
							Composite{
								Layout: HBox{MarginsZero: true},
								Children: []Widget{
									PushButton{Text: "Add >>", OnClicked: func() { wa.addSelected() }},
									PushButton{Text: "<< Remove", OnClicked: func() { wa.removeSelected() }},
									HSpacer{},
									PushButton{Text: "Clear All", OnClicked: func() {
										wa.selectedIDs = make(map[int]bool)
										wa.refreshLists()
										wa.refreshHexTemplate()
									}},
									PushButton{
										Text: "→ Go to Calculate Mask tab",
										OnClicked: func() {
											wa.tw.SetCurrentIndex(2)
										},
									},
								},
							},
						},
					},

					// ══════════════════════════════════════════════════════
					// Tab 3 — Calculate Mask
					// ══════════════════════════════════════════════════════
					{
						Title:  "  Calculate Mask  ",
						Layout: VBox{Margins: Margins{Left: 8, Top: 8, Right: 8, Bottom: 8}},
						Children: []Widget{

							// Step 2 — per-group hex input
							GroupBox{
								Title:  "Step 2 — Current hex values from CAFD  (default FF = all bits masked)",
								Layout: VBox{},
								Children: []Widget{
									Composite{
										Layout: HBox{MarginsZero: true},
										Children: []Widget{
											PushButton{
												Text:      "Load from CAFD / NCD file…",
												OnClicked: func() { wa.loadCAFD() },
											},
											PushButton{
												Text:      "Reset all to FF",
												OnClicked: func() { wa.resetHexToFF() },
											},
										},
									},
									// Per-group rows are added imperatively here.
									Composite{
										AssignTo: &wa.hexComp,
										Layout:   VBox{MarginsZero: true},
										MinSize:  Size{Height: 20},
									},
								},
							},

							PushButton{
								Text:      "▶  CALCULATE",
								OnClicked: func() { wa.calculate() },
							},

							// Step 3 — per-group results with Copy buttons
							GroupBox{
								Title:  "Step 3 — Results",
								Layout: VBox{},
								Children: []Widget{
									// Per-group result rows are added imperatively here.
									Composite{
										AssignTo: &wa.resComp,
										Layout:   VBox{MarginsZero: true},
										MinSize:  Size{Height: 20},
									},
								},
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

	// Load the application icon from the embedded exe resources (resource ID 1 = icon.ico).
	// This sets the icon in the title bar and taskbar.  The exe file icon in Explorer and
	// the taskbar button are handled automatically by Windows from the same embedded resource.
	if icon, err := walk.NewIconFromResourceId(1); err == nil {
		wa.mw.SetIcon(icon)
	}

	go checkAndUpdate(wa.mw)

	// Auto-search is checked by default — start immediately after window is created.
	wa.leVCIHost.SetReadOnly(true)
	wa.startAutoSearch()

	wa.mw.Run()
}

// ── Tab 1: Auto-search (BMW ZGW discovery) ───────────────────────────────────

// startAutoSearch launches bmwzgw.Run in a background goroutine.
// When the ZGW gateway is found, the VCI IP field is updated automatically.
func (a *winApp) startAutoSearch() {
	ctx, cancel := context.WithCancel(context.Background())
	a.zgwCancel = cancel
	a.leZGW.SetText("Searching for BMW on 169.254.x.x…")

	go bmwzgw.Run(ctx, "", func(info *bmwzgw.Info) {
		a.mw.Synchronize(func() {
			if !a.cbAutoSearch.Checked() {
				return // checkbox was unchecked while goroutine was still running
			}
			if info == nil {
				a.leZGW.SetText("No BMW ENET adapter found (169.254.x.x interface not detected)")
				return
			}
			if info.VIN == "" {
				// Stage 1: 169.254.x.x interface detected, ZGW has not yet responded.
				// Don't update the IP field yet — it's the host's own IP, not the ZGW's.
				a.leZGW.SetText(fmt.Sprintf(
					"ENET adapter detected (%s) — waiting for ZGW response…", info.IP))
			} else {
				// Stage 2: ZGW responded — info.IP is the gateway's real IP.
				a.leVCIHost.SetText(info.IP)
				s := info.IP
				if info.Model != "" {
					s += "  " + info.Model
				}
				s += "  " + info.VIN
				if info.Target != "" {
					s += "  " + info.Target
				}
				a.leZGW.SetText(s)
			}
		})
	})
}

// stopAutoSearch cancels the running ZGW discovery goroutine.
func (a *winApp) stopAutoSearch() {
	if a.zgwCancel != nil {
		a.zgwCancel()
		a.zgwCancel = nil
	}
}

// ── Tab 1: Read from Car ──────────────────────────────────────────────────────

func (a *winApp) readFromCar() {
	host := strings.TrimSpace(a.leVCIHost.Text())
	if host == "" {
		walk.MsgBox(a.mw, "No IP", "Enter the VCI IP address.", walk.MsgBoxIconWarning)
		return
	}
	a.lblLive.SetText("Connecting…")
	a.lbLive.SetModel([]string{})
	a.liveCCIDs = nil

	go func() {
		ccids, err := ReadVehicleCCIDs(host)
		a.mw.Synchronize(func() {
			if err != nil {
				// Truncate very long error messages so they don't break the layout.
				msg := "Error — " + err.Error()
				if len(msg) > 120 {
					msg = msg[:117] + "…"
				}
				a.lblLive.SetText(msg)
				return
			}
			a.liveCCIDs = ccids
			if len(ccids) == 0 {
				a.lblLive.SetText("Connected — no active CC-IDs found in cluster")
				return
			}
			a.refreshLiveList()
			a.lblLive.SetText(fmt.Sprintf("%d CC-ID(s) found — double-click to add", len(ccids)))
		})
	}()
}

// addFromCar adds the double-clicked CC-ID from the live list to the selected set.
func (a *winApp) addFromCar() {
	idx := a.lbLive.CurrentIndex()
	if idx < 0 || idx >= len(a.liveCCIDs) {
		return
	}
	c := a.liveCCIDs[idx]
	a.selectedIDs[c.ID] = true
	a.refreshLists()
	a.refreshHexTemplate()
	a.refreshLiveList()
	a.lblLive.SetText(fmt.Sprintf("Added CC-ID %d — %d selected total", c.ID, len(a.selectedIDs)))
}

// addAllFromCar adds every CC-ID returned from the car and switches to Tab 2.
func (a *winApp) addAllFromCar() {
	if len(a.liveCCIDs) == 0 {
		walk.MsgBox(a.mw, "Nothing to add",
			"Connect to the car first.", walk.MsgBoxIconWarning)
		return
	}
	for _, c := range a.liveCCIDs {
		a.selectedIDs[c.ID] = true
	}
	a.refreshLists()
	a.refreshHexTemplate()
	a.refreshLiveList()
	a.lblLive.SetText(fmt.Sprintf("All %d CC-IDs added (%d selected total)",
		len(a.liveCCIDs), len(a.selectedIDs)))
	a.tw.SetCurrentIndex(1) // jump to Select tab
}

// setDescMode changes the active language/mode and keeps both ComboBoxes in sync.
// Safe to call from either ComboBox's OnCurrentIndexChanged handler.
func (a *winApp) setDescMode(idx int) {
	if a.syncingDescMode {
		return
	}
	a.syncingDescMode = true
	a.descMode = idx
	if a.cbDescMode != nil {
		a.cbDescMode.SetCurrentIndex(idx)
	}
	if a.cbDescMode2 != nil {
		a.cbDescMode2.SetCurrentIndex(idx)
	}
	a.syncingDescMode = false
	a.refreshLists()
	a.refreshLiveList()
}

// refreshLiveList redraws the live ListBox with updated checkmarks.
func (a *winApp) refreshLiveList() {
	if len(a.liveCCIDs) == 0 {
		return
	}
	entryByID := make(map[int]CCIDEntry, len(a.allEntries))
	for _, e := range a.allEntries {
		entryByID[e.ID] = e
	}
	items := make([]string, len(a.liveCCIDs))
	for i, c := range a.liveCCIDs {
		mark := "   "
		if a.selectedIDs[c.ID] {
			mark = " ✓ "
		}
		desc := c.Description
		if e, ok := entryByID[c.ID]; ok {
			desc = a.entryDesc(e)
		}
		items[i] = fmt.Sprintf("%s%-5d  %s", mark, c.ID, desc)
	}
	a.lbLive.SetModel(items)
}

// ── Tab 2: CC-ID list helpers ─────────────────────────────────────────────────

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
		items[i] = fmt.Sprintf("%-5d  %s", e.ID, a.entryDesc(e))
	}
	a.lbAvailable.SetModel(items)

	sel := a.getSelected()
	selItems := make([]string, len(sel))
	for i, e := range sel {
		selItems[i] = fmt.Sprintf("%-5d  %s", e.ID, a.entryDesc(e))
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
		found := false
		for _, ae := range a.allEntries {
			if ae.ID == id {
				entries = append(entries, ae) // full entry — all language fields
				found = true
				break
			}
		}
		if !found {
			entries = append(entries, CCIDEntry{ID: id, Description: fmt.Sprintf("CC-ID %d", id)})
		}
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].ID < entries[j].ID })
	return entries
}

// ── Tab 3: Hex input & calculation ───────────────────────────────────────────

// refreshHexTemplate rebuilds the per-group hex input rows, preserving any
// values the user has already typed.
func (a *winApp) refreshHexTemplate() {
	// Save current typed values before we dispose the fields.
	existing := make(map[int][]byte)
	for gn, le := range a.hexFields {
		if b, err := parseHexLine(le.Text()); err == nil {
			existing[gn] = b
		}
	}
	a.hexFields = make(map[int]*walk.LineEdit)

	// Batch all child widget changes to avoid flicker.
	a.hexComp.SetSuspended(true)

	// Remove every current child row.
	ch := a.hexComp.Children()
	for i := ch.Len() - 1; i >= 0; i-- {
		ch.At(i).Dispose()
	}

	for _, gn := range a.affectedGroups() {
		// Start with all-FF; restore previously typed value if available.
		var b [8]byte
		for i := range b {
			b[i] = 0xFF
		}
		if ex, ok := existing[gn]; ok && len(ex) == 8 {
			copy(b[:], ex)
		}

		row, _ := walk.NewComposite(a.hexComp)
		hl := walk.NewHBoxLayout()
		hl.SetMargins(walk.Margins{})
		hl.SetSpacing(6)
		row.SetLayout(hl)

		lbl, _ := walk.NewLineEdit(row)
		lbl.SetText(fmt.Sprintf("CC_AKTIVIERUNG_%d:", gn))
		lbl.SetReadOnly(true)
		lbl.SetMinMaxSize(walk.Size{Width: 170}, walk.Size{Width: 170})

		le, _ := walk.NewLineEdit(row)
		le.SetText(bytesToHexComma(b[:]))
		a.hexFields[gn] = le
	}

	a.hexComp.SetSuspended(false)
}

// resetHexToFF resets every group's hex LineEdit back to all-FF, discarding
// any values loaded from a CAFD/NCD file or typed by the user.
func (a *winApp) resetHexToFF() {
	ff := bytesToHexComma([]byte{0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF})
	for _, le := range a.hexFields {
		le.SetText(ff)
	}
}

// loadCAFD lets the user pick a CAFD / NCD / S-record file and updates
// the hex LineEdits with the values read from that file.
func (a *winApp) loadCAFD() {
	dlg := new(walk.FileDialog)
	dlg.Title = "Load CAFD / NCD / S-record file"
	dlg.Filter = "CAFD/NCD/S-record (*.cafd;*.ncd;*.sre;*.s19;*.srec;*.txt)|*.cafd;*.ncd;*.sre;*.s19;*.srec;*.txt|All files (*.*)|*.*"
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
	// Update only the groups that are already shown (i.e. have a selected CC-ID).
	for gn, b := range cafdData {
		if le, ok := a.hexFields[gn]; ok && len(b) == 8 {
			le.SetText(bytesToHexComma(b))
		}
	}
}

// calculate reads hex values from the LineEdits, applies the CC-ID mask, then
// rebuilds the results composite with per-group rows and individual Copy buttons.
func (a *winApp) calculate() {
	if len(a.selectedIDs) == 0 {
		walk.MsgBox(a.mw, "Nothing selected",
			"Please select at least one CC-ID on the \"Select CC-IDs\" tab first.",
			walk.MsgBoxIconWarning)
		return
	}

	// Build initial states from the per-group LineEdits.
	initialStates := make(map[int][]byte)
	for _, gn := range a.affectedGroups() {
		b := make([]byte, 8)
		for i := range b {
			b[i] = 0xFF
		}
		if le, ok := a.hexFields[gn]; ok {
			if parsed, err := parseHexLine(le.Text()); err == nil {
				b = parsed
			}
		}
		initialStates[gn] = b
	}

	ids := make([]int, 0, len(a.selectedIDs))
	for id := range a.selectedIDs {
		ids = append(ids, id)
	}
	results := calculateMask(initialStates, ids)

	// Rebuild the results composite.
	a.resComp.SetSuspended(true)
	ch := a.resComp.Children()
	for i := ch.Len() - 1; i >= 0; i-- {
		ch.At(i).Dispose()
	}

	for _, gr := range results {
		hexStr := bytesToHexComma(gr.ModifiedBytes)

		row, _ := walk.NewComposite(a.resComp)
		hl := walk.NewHBoxLayout()
		hl.SetMargins(walk.Margins{})
		hl.SetSpacing(6)
		row.SetLayout(hl)

		lbl, _ := walk.NewLineEdit(row)
		lbl.SetText(fmt.Sprintf("CC_AKTIVIERUNG_%d", gr.GroupNum))
		lbl.SetReadOnly(true)
		lbl.SetMinMaxSize(walk.Size{Width: 170}, walk.Size{Width: 170})

		valLe, _ := walk.NewLineEdit(row)
		valLe.SetText(hexStr)
		valLe.SetReadOnly(true)

		// Capture hexStr by value so the closure is correct for each iteration.
		capturedHex := hexStr
		btn, _ := walk.NewPushButton(row)
		btn.SetText("Copy")
		btn.Clicked().Attach(func() {
			walk.Clipboard().SetText(capturedHex)
		})
	}

	a.resComp.SetSuspended(false)
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

// ── description mode ─────────────────────────────────────────────────────────

var descModeNames = []string{
	"EnUS",
	"EnUS_LongText",
	"DeDe",
	"DeDe_LongText",
	"EnGB",
	"EnGB_LongText",
}

// entryDesc returns the display string for e using the current language/mode.
// Falls back to the next available language when the chosen field is empty.
func (a *winApp) entryDesc(e CCIDEntry) string {
	switch a.descMode {
	case 0:
		return firstNonEmpty(e.TitleENUS, e.TitleENGB, e.TitleDEDE)
	case 1:
		return firstNonEmpty(e.LongENUS, e.TitleENUS, e.LongENGB)
	case 2:
		return firstNonEmpty(e.TitleDEDE, e.TitleENGB, e.TitleENUS)
	case 3:
		return firstNonEmpty(e.LongDEDE, e.TitleDEDE, e.LongENGB)
	case 4:
		return firstNonEmpty(e.TitleENGB, e.TitleENUS, e.TitleDEDE)
	case 5:
		return firstNonEmpty(e.LongENGB, e.TitleENGB, e.LongENUS)
	}
	return e.Description
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// ── hex helpers ───────────────────────────────────────────────────────────────

// bytesToHexComma converts a byte slice to "FF, FF, FF, FF, FF, FF, FF, FF".
func bytesToHexComma(b []byte) string {
	parts := make([]string, len(b))
	for i, v := range b {
		parts[i] = fmt.Sprintf("%02X", v)
	}
	return strings.Join(parts, ", ")
}

// parseHexLine parses a hex string with comma or space separators into exactly
// 8 bytes. Returns an error if the format is wrong.
func parseHexLine(s string) ([]byte, error) {
	s = strings.ReplaceAll(s, ",", " ")
	parts := strings.Fields(s)
	if len(parts) != 8 {
		return nil, fmt.Errorf("expected 8 hex bytes, got %d tokens", len(parts))
	}
	b := make([]byte, 8)
	for i, p := range parts {
		v, err := strconv.ParseUint(strings.ToUpper(p), 16, 8)
		if err != nil {
			return nil, fmt.Errorf("invalid hex %q at position %d", p, i)
		}
		b[i] = byte(v)
	}
	return b, nil
}
