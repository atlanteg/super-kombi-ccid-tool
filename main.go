package main

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
)

func main() {
	a := app.NewWithID("com.bmwtools.ccid-calculator")
	w := a.NewWindow("BMW Kombi CC-ID Calculator")
	w.Resize(fyne.NewSize(960, 620))

	ui := newCCIDApp(a, w)
	ui.showStep1()

	w.ShowAndRun()
}
