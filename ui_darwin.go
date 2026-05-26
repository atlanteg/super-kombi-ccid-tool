//go:build darwin

package main

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

const defaultVCIHost = "169.254.138.176"

type ccidApp struct {
	fyneApp fyne.App
	win     fyne.Window

	allEntries  []CCIDEntry
	descs       map[int]string
	selectedIDs map[int]bool
	filtered    []CCIDEntry

	hexInputs map[int][8]*widget.Entry
}

func newCCIDApp(a fyne.App, w fyne.Window) *ccidApp {
	all := loadAllEntries()
	descs := loadDescriptions()
	app := &ccidApp{
		fyneApp:     a,
		win:         w,
		allEntries:  all,
		descs:       descs,
		selectedIDs: make(map[int]bool),
		filtered:    all,
		hexInputs:   make(map[int][8]*widget.Entry),
	}
	return app
}

// ── Step 1 ────────────────────────────────────────────────────────────────────

func (a *ccidApp) showStep1() {
	a.filtered = a.allEntries

	selEntries := func() []CCIDEntry { return a.getSelected() }

	selHeader := widget.NewLabelWithStyle(
		fmt.Sprintf("Selected (%d)", len(a.selectedIDs)),
		fyne.TextAlignLeading, fyne.TextStyle{Bold: true},
	)

	var selList *widget.List

	availList := widget.NewList(
		func() int { return len(a.filtered) },
		func() fyne.CanvasObject {
			return container.NewBorder(nil, nil, nil,
				widget.NewButton("→", nil),
				widget.NewLabel("template"),
			)
		},
		func(id widget.ListItemID, obj fyne.CanvasObject) {
			c := obj.(*fyne.Container)
			lbl := c.Objects[0].(*widget.Label)
			btn := c.Objects[1].(*widget.Button)
			if id >= len(a.filtered) {
				return
			}
			e := a.filtered[id]
			lbl.SetText(fmt.Sprintf("%-4d  %s", e.ID, e.Description))
			btn.OnTapped = func() {
				a.selectedIDs[e.ID] = true
				selList.Refresh()
				selHeader.SetText(fmt.Sprintf("Selected (%d)", len(a.selectedIDs)))
			}
		},
	)

	selList = widget.NewList(
		func() int { return len(selEntries()) },
		func() fyne.CanvasObject {
			return container.NewBorder(nil, nil, nil,
				widget.NewButton("✕", nil),
				widget.NewLabel("template"),
			)
		},
		func(id widget.ListItemID, obj fyne.CanvasObject) {
			entries := selEntries()
			if id >= len(entries) {
				return
			}
			c := obj.(*fyne.Container)
			lbl := c.Objects[0].(*widget.Label)
			btn := c.Objects[1].(*widget.Button)
			e := entries[id]
			lbl.SetText(fmt.Sprintf("%-4d  %s", e.ID, e.Description))
			btn.OnTapped = func() {
				delete(a.selectedIDs, e.ID)
				selList.Refresh()
				selHeader.SetText(fmt.Sprintf("Selected (%d)", len(a.selectedIDs)))
			}
		},
	)

	search := widget.NewEntry()
	search.SetPlaceHolder("Search by number or description…")
	search.OnChanged = func(raw string) {
		q := strings.ToLower(strings.TrimSpace(raw))
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
		availList.Refresh()
	}

	left := container.NewBorder(
		container.NewVBox(
			widget.NewLabelWithStyle("Available CC-IDs", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
			search,
		),
		nil, nil, nil, availList,
	)
	right := container.NewBorder(selHeader, nil, nil, nil, selList)

	split := container.NewHSplit(left, right)
	split.SetOffset(0.6)

	clearBtn := widget.NewButton("Clear All", func() {
		a.selectedIDs = make(map[int]bool)
		selList.Refresh()
		selHeader.SetText("Selected (0)")
	})
	clearBtn.Importance = widget.LowImportance

	readCarBtn := widget.NewButton("🔌 Read from Car…", func() {
		a.showReadFromCar()
	})

	nextBtn := widget.NewButton("Next: Enter Hex Values →", func() {
		if len(a.selectedIDs) == 0 {
			dialog.ShowInformation("Nothing selected", "Please select at least one CC-ID.", a.win)
			return
		}
		a.showStep2()
	})
	nextBtn.Importance = widget.HighImportance

	title := widget.NewLabelWithStyle(
		"Step 1 of 3 — Select CC-IDs to Activate",
		fyne.TextAlignCenter, fyne.TextStyle{Bold: true},
	)
	bottomBar := container.NewBorder(nil, nil,
		container.NewHBox(clearBtn, readCarBtn),
		nextBtn,
	)
	a.win.SetContent(container.NewBorder(
		container.NewVBox(title, widget.NewSeparator()),
		container.NewVBox(widget.NewSeparator(), bottomBar),
		nil, nil, split,
	))
}

func (a *ccidApp) getSelected() []CCIDEntry {
	entries := make([]CCIDEntry, 0, len(a.selectedIDs))
	for id := range a.selectedIDs {
		desc := a.descs[id]
		if desc == "" {
			desc = "No description"
		}
		entries = append(entries, CCIDEntry{ID: id, Description: desc})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].ID < entries[j].ID })
	return entries
}

func (a *ccidApp) affectedGroups() []int {
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

// ── Step 2 ────────────────────────────────────────────────────────────────────

func (a *ccidApp) showStep2() {
	a.hexInputs = make(map[int][8]*widget.Entry)
	groups := a.affectedGroups()

	groupsBox := container.NewVBox()
	for _, gn := range groups {
		gnCopy := gn
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

		hdr := widget.NewLabelWithStyle(
			fmt.Sprintf("Group %d  (CC-IDs %d–%d)  →  activating: %s",
				gn, minID, maxID, strings.Join(idStrs, ", ")),
			fyne.TextAlignLeading, fyne.TextStyle{Bold: true},
		)

		var entries [8]*widget.Entry
		entriesRow := make([]fyne.CanvasObject, 8)
		for i := 0; i < 8; i++ {
			e := widget.NewEntry()
			e.SetText("FF")
			e.SetPlaceHolder("FF")
			idx := i
			e.OnChanged = func(s string) {
				u := strings.ToUpper(s)
				if u != s {
					e.SetText(u)
				}
				if len(u) > 2 {
					e.SetText(u[:2])
				}
				_ = idx
			}
			entries[i] = e
			entriesRow[i] = e
		}
		a.hexInputs[gnCopy] = entries

		byteGrid := container.New(layout.NewGridLayoutWithColumns(8), entriesRow...)
		groupsBox.Add(container.NewVBox(hdr, byteGrid, widget.NewSeparator()))
	}

	loadBtn := widget.NewButton("Load from CAFD file…", func() {
		dialog.ShowFileOpen(func(r fyne.URIReadCloser, err error) {
			if err != nil || r == nil {
				return
			}
			defer r.Close()
			cafdData, err := parseCAFDFile(r.URI().Path())
			if err != nil || cafdData == nil {
				dialog.ShowError(fmt.Errorf("cannot parse CAFD: %v", err), a.win)
				return
			}
			for groupNum, entries := range a.hexInputs {
				if b, ok := cafdData[groupNum]; ok && len(b) == 8 {
					for i, e := range entries {
						e.SetText(fmt.Sprintf("%02X", b[i]))
					}
				}
			}
		}, a.win)
	})

	backBtn := widget.NewButton("← Back", func() { a.showStep1() })
	calcBtn := widget.NewButton("Calculate →", func() { a.calculate() })
	calcBtn.Importance = widget.HighImportance

	hint := widget.NewLabel("Enter current hex bytes from your CAFD file, or leave as FF (all masked).")
	hint.Wrapping = fyne.TextWrapWord

	title := widget.NewLabelWithStyle(
		"Step 2 of 3 — Enter Current Hex Values",
		fyne.TextAlignCenter, fyne.TextStyle{Bold: true},
	)
	a.win.SetContent(container.NewBorder(
		container.NewVBox(title, widget.NewSeparator(), hint, loadBtn, widget.NewSeparator()),
		container.NewVBox(widget.NewSeparator(), container.NewBorder(nil, nil, backBtn, calcBtn)),
		nil, nil,
		container.NewScroll(groupsBox),
	))
}

func (a *ccidApp) calculate() {
	initialStates := make(map[int][]byte)
	for groupNum, entries := range a.hexInputs {
		b := make([]byte, 8)
		for i, e := range entries {
			text := strings.TrimSpace(strings.ToUpper(e.Text))
			if text == "" {
				text = "FF"
			}
			val, err := strconv.ParseUint(text, 16, 8)
			if err != nil {
				dialog.ShowError(fmt.Errorf(
					"Group %d, byte %d: invalid hex %q", groupNum, i+1, text,
				), a.win)
				return
			}
			b[i] = byte(val)
		}
		initialStates[groupNum] = b
	}
	ids := make([]int, 0, len(a.selectedIDs))
	for id := range a.selectedIDs {
		ids = append(ids, id)
	}
	a.showStep3(calculateMask(initialStates, ids))
}

// ── Step 3 ────────────────────────────────────────────────────────────────────

func (a *ccidApp) showStep3(results []*GroupResult) {
	content := container.NewVBox()
	var allLines []string

	for _, gr := range results {
		grCopy := gr
		origStr := bytesToHex(gr.OriginalBytes)
		modStr := bytesToHex(gr.ModifiedBytes)

		var changes []string
		for _, idx := range gr.ModifiedIndices {
			changes = append(changes, fmt.Sprintf(
				"byte %d: %02X→%02X", idx+1, gr.OriginalBytes[idx], gr.ModifiedBytes[idx],
			))
		}

		hdr := widget.NewLabelWithStyle(
			fmt.Sprintf("Group %d  (CC-IDs %d–%d)",
				gr.GroupNum, (gr.GroupNum-1)*64, gr.GroupNum*64-1),
			fyne.TextAlignLeading, fyne.TextStyle{Bold: true},
		)
		origLbl := monoLabel("Before: " + origStr)
		modLbl := monoLabel("After:  " + modStr)
		chgLbl := widget.NewLabel("Changes: " + strings.Join(changes, ",  "))

		copyBtn := widget.NewButtonWithIcon("Copy After", theme.ContentCopyIcon(), func() {
			a.win.Clipboard().SetContent(bytesToHex(grCopy.ModifiedBytes))
		})

		content.Add(widget.NewCard("", "", container.NewVBox(hdr, origLbl, modLbl, chgLbl, copyBtn)))
		allLines = append(allLines, fmt.Sprintf("Group %d: %s", gr.GroupNum, modStr))
	}

	copyAllBtn := widget.NewButtonWithIcon("Copy All Results", theme.ContentCopyIcon(), func() {
		a.win.Clipboard().SetContent(strings.Join(allLines, "\n"))
	})
	copyAllBtn.Importance = widget.HighImportance

	startOverBtn := widget.NewButton("← Start Over", func() {
		a.selectedIDs = make(map[int]bool)
		a.hexInputs = make(map[int][8]*widget.Entry)
		a.filtered = a.allEntries
		a.showStep1()
	})

	title := widget.NewLabelWithStyle(
		"Results — Modified CC-ID Masks",
		fyne.TextAlignCenter, fyne.TextStyle{Bold: true},
	)
	a.win.SetContent(container.NewBorder(
		container.NewVBox(title, widget.NewSeparator()),
		container.NewVBox(widget.NewSeparator(), container.NewBorder(nil, nil, startOverBtn, copyAllBtn)),
		nil, nil,
		container.NewScroll(content),
	))
}

// ── Read from Car ─────────────────────────────────────────────────────────────

func (a *ccidApp) showReadFromCar() {
	ipEntry := widget.NewEntry()
	ipEntry.SetText(defaultVCIHost)
	ipEntry.SetPlaceHolder("169.254.x.x")

	statusLbl := widget.NewLabel("Enter the VCI IP address and press Connect.")
	statusLbl.Wrapping = fyne.TextWrapWord

	resultList := widget.NewList(
		func() int { return 0 },
		func() fyne.CanvasObject { return widget.NewLabel("") },
		func(_ widget.ListItemID, _ fyne.CanvasObject) {},
	)

	var liveResults []LiveCCID

	connectBtn := widget.NewButton("Connect & Read", func() {
		host := strings.TrimSpace(ipEntry.Text)
		if host == "" {
			statusLbl.SetText("Please enter the VCI IP address.")
			return
		}
		statusLbl.SetText("Connecting to " + host + ":6801 …")
		go func() {
			ccids, err := ReadVehicleCCIDs(host)
			if err != nil {
				statusLbl.SetText("Error: " + err.Error())
				return
			}
			liveResults = ccids
			resultList.Length = func() int { return len(liveResults) }
			resultList.CreateItem = func() fyne.CanvasObject {
				return container.NewBorder(nil, nil,
					widget.NewLabelWithStyle("0000", fyne.TextAlignLeading, fyne.TextStyle{Monospace: true}),
					nil,
					widget.NewLabel("description"),
				)
			}
			resultList.UpdateItem = func(id widget.ListItemID, obj fyne.CanvasObject) {
				if id >= len(liveResults) {
					return
				}
				c := obj.(*fyne.Container)
				c.Objects[0].(*widget.Label).SetText(fmt.Sprintf("%4d", liveResults[id].ID))
				c.Objects[1].(*widget.Label).SetText(liveResults[id].Description)
			}
			if len(ccids) == 0 {
				statusLbl.SetText("Connected — no active CC-IDs found.")
			} else {
				statusLbl.SetText(fmt.Sprintf("Found %d stored CC-ID(s):", len(ccids)))
			}
			resultList.Refresh()
		}()
	})
	connectBtn.Importance = widget.HighImportance

	backBtn := widget.NewButton("← Back", func() { a.showStep1() })

	title := widget.NewLabelWithStyle(
		"Read CC-IDs from Connected Car",
		fyne.TextAlignCenter, fyne.TextStyle{Bold: true},
	)
	hint := widget.NewLabel(
		"Requires BMW VCI (ISTA-compatible) connected via OBD-II.\n" +
			"Car ignition must be ON. Uses EDIABAS TCP port 6801.",
	)
	hint.Wrapping = fyne.TextWrapWord

	topRow := container.NewBorder(nil, nil, widget.NewLabel("VCI IP:"), connectBtn, ipEntry)

	a.win.SetContent(container.NewBorder(
		container.NewVBox(title, widget.NewSeparator(), hint, topRow, widget.NewSeparator(), statusLbl),
		container.NewVBox(widget.NewSeparator(), container.NewBorder(nil, nil, backBtn, nil)),
		nil, nil,
		container.NewScroll(resultList),
	))
}

// ── helpers ───────────────────────────────────────────────────────────────────

func bytesToHex(b []byte) string {
	parts := make([]string, len(b))
	for i, v := range b {
		parts[i] = fmt.Sprintf("%02X", v)
	}
	return strings.Join(parts, " ")
}

func monoLabel(text string) *widget.Label {
	l := widget.NewLabel(text)
	l.TextStyle = fyne.TextStyle{Monospace: true}
	return l
}
