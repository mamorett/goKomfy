package main

import (
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/theme"
	"github.com/mamorett/goKomfy/internal/ui"
)

func main() {
	a := app.NewWithID("com.github.mamorett.goKomfy")
	a.SetIcon(theme.FileApplicationIcon())
	w := ui.NewMainWindow(a)
	w.ShowAndRun()
}
