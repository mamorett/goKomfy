package main

import (
	"flag"
	"os"
	"runtime"

	"fyne.io/fyne/v2/app"
	"github.com/mamorett/goKomfy/internal/ui"
)

func main() {
	debugGL := flag.Bool("debug-gl", false, "Enable GL debug logging")
	flag.Parse()

	if *debugGL {
		os.Setenv("FYNE_DEBUG", "1")
	}

	// Linux-specific stability environment variables
	if runtime.GOOS == "linux" {
		// Force X11 backend if not set, as it is more stable for Fyne/GLFW on many distros
		if os.Getenv("GDK_BACKEND") == "" {
			os.Setenv("GDK_BACKEND", "x11")
		}
	}

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
