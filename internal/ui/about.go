package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/dialog"
)

func showAbout(win fyne.Window) {
	content := `ComfyUI Prompt Extractor
Version 4.1 (Stability Patch)

A tool to extract positive prompts from ComfyUI-generated PNG files.

Features:
• Dual extraction modes (ComfyUI / Parameters)
• Drag & drop interface
• Image thumbnails
• Export to text file

Keyboard Shortcuts:
• Ctrl+O: Open file(s)
• Ctrl+E: Toggle extraction mode
• Ctrl+C: Copy all prompts
• Ctrl+S: Save to file
• Ctrl+L: Clear results`
	dialog.ShowInformation("About", content, win)
}
