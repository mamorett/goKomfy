package main

import (
	"fyne.io/fyne/v2/app"
	"github.com/mamorett/goKomfy/internal/ui"
)

func main() {
	a := app.NewWithID("gokomfy")
	a.SetIcon(resourceLogoPng)
	w := ui.NewMainWindow(a)
	w.ShowAndRun()
}
