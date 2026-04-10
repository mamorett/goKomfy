package main

import (
	"os"

	"fyne.io/fyne/v2/app"
	"github.com/mamorett/goKomfy/internal/ui"
)

func main() {
	// Set RESOURCE_NAME before the GLFW/X11 backend initialises.
	// GLFW uses this env var as both the WM_CLASS instance name and class name,
	// which allows KDE Plasma, Plank, and other dock/taskbar software to
	// correctly associate the running window with the gokomfy.desktop entry.
	if os.Getenv("RESOURCE_NAME") == "" {
		os.Setenv("RESOURCE_NAME", "gokomfy")
	}

	a := app.NewWithID("gokomfy")
	a.SetIcon(resourceLogoPng)
	w := ui.NewMainWindow(a)
	w.ShowAndRun()
}
