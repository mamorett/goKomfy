package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"
	"image/color"
)

func showShortcuts(win fyne.Window) {
	data := [][]string{
		{"Open File(s)", "Ctrl+O"},
		{"Toggle Mode", "Ctrl+E"},
		{"Save to File", "Ctrl+S"},
		{"Clear Results", "Ctrl+L"},
		{"Copy to Clipboard", "Ctrl+C"},
		{"About", "F1"},
		{"Shortcuts", "?"},
	}

	table := widget.NewTable(
		func() (int, int) { return len(data), 2 },
		func() fyne.CanvasObject {
			return widget.NewLabel("Shortcuts")
		},
		func(id widget.TableCellID, o fyne.CanvasObject) {
			l := o.(*widget.Label)
			l.SetText(data[id.Row][id.Col])
			if id.Col == 1 {
				l.TextStyle = fyne.TextStyle{Monospace: true, Bold: true}
				l.Alignment = fyne.TextAlignTrailing
			} else {
				l.TextStyle = fyne.TextStyle{Bold: true}
			}
		},
	)
	table.SetColumnWidth(0, 200)
	table.SetColumnWidth(1, 150)

	title := widget.NewLabelWithStyle("Keyboard Shortcuts", fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
	
	bg := canvas.NewRectangle(color.RGBA{R: 0, G: 0, B: 0, A: 200})
	content := container.NewPadded(container.NewBorder(title, nil, nil, nil, table))
	
	closeBtn := widget.NewButton("Close", nil)
	
	// Create a pop-up or a dialog
	d := widget.NewModalPopUp(container.NewStack(bg, container.NewBorder(nil, closeBtn, nil, nil, content)), win.Canvas())
	closeBtn.OnTapped = func() {
		d.Hide()
	}
	d.Resize(fyne.NewSize(400, 400))
	d.Show()
}

func (mw *MainWindow) setupShortcuts() {
	// Help / Shortcuts Reference
	mw.window.Canvas().SetOnTypedKey(func(k *fyne.KeyEvent) {
		if k.Name == fyne.KeySlash {
			showShortcuts(mw.window)
		}
	})

	mw.window.Canvas().AddShortcut(&desktop.CustomShortcut{
		KeyName:  fyne.KeyO,
		Modifier: fyne.KeyModifierControl,
	}, func(s fyne.Shortcut) {
		mw.browseFiles()
	})

	mw.window.Canvas().AddShortcut(&desktop.CustomShortcut{
		KeyName:  fyne.KeyE,
		Modifier: fyne.KeyModifierControl,
	}, func(s fyne.Shortcut) {
		mw.toggleMode()
	})

	mw.window.Canvas().AddShortcut(&desktop.CustomShortcut{
		KeyName:  fyne.KeyS,
		Modifier: fyne.KeyModifierControl,
	}, func(s fyne.Shortcut) {
		mw.saveToFile()
	})

	mw.window.Canvas().AddShortcut(&desktop.CustomShortcut{
		KeyName:  fyne.KeyL,
		Modifier: fyne.KeyModifierControl,
	}, func(s fyne.Shortcut) {
		mw.clearResults()
	})
    
    mw.window.Canvas().AddShortcut(&desktop.CustomShortcut{
		KeyName:  fyne.KeyF1,
	}, func(s fyne.Shortcut) {
		showAbout(mw.window)
	})
}
