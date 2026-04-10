package ui

import (
	"bufio"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/atotto/clipboard"
	"github.com/mamorett/goKomfy/internal/extractor"
	"golang.org/x/image/draw"
)

type AppState struct {
	currentFiles    []string
	currentResults  []*extractor.ExtractionResult
	allPromptTexts  []string
	mode            string // "ComfyUI" or "Parameters"
	busy            bool
	autoCopy        bool
}

type MainWindow struct {
	app    fyne.App
	window fyne.Window

	state AppState

	modeSelect    *widget.Select
	autoCopyCheck *widget.Check
	promptEntry   *ReadOnlyEntry
	summaryEntry  *ReadOnlyEntry
	progressBar   *widget.ProgressBarInfinite
	
	dropZone     *DropZone
	previewImg   *canvas.Image
	previewLabel *widget.Label
	previewCont  *fyne.Container

	copyAllBtn   *widget.Button
	copyFirstBtn *widget.Button
	saveBtn      *widget.Button
	clearBtn     *widget.Button
	statusLabel  *widget.Label
}

func NewMainWindow(a fyne.App) *MainWindow {
	mw := &MainWindow{
		app: a,
		state: AppState{
			mode: "ComfyUI",
		},
	}
	mw.window = a.NewWindow("goKomfy — Prompt Extractor")
	mw.window.Resize(fyne.NewSize(1000, 800))

	mw.setupUI()
	mw.setupShortcuts()
	mw.setupMenu()

	return mw
}

func (mw *MainWindow) setupUI() {
	// 1. Header (Mode + Browse)
	mw.modeSelect = widget.NewSelect([]string{"ComfyUI", "Parameters"}, func(s string) {
		mw.state.mode = s
		// Re-extract if we already have files loaded
		if len(mw.state.currentFiles) > 0 && !mw.state.busy {
			mw.processFiles(mw.state.currentFiles)
		}
	})
	mw.modeSelect.SetSelected(mw.state.mode)

	mw.autoCopyCheck = widget.NewCheck("Auto-copy to Clipboard", func(b bool) {
		mw.state.autoCopy = b
	})
	
	browseFilesBtn := widget.NewButtonWithIcon("Open Files", theme.FileIcon(), func() {
		mw.browseFiles()
	})
	browseFolderBtn := widget.NewButtonWithIcon("Open Folder", theme.FolderOpenIcon(), func() {
		mw.browseFolder()
	})

	header := container.NewHBox(
		widget.NewLabelWithStyle("goKomfy", fyne.TextAlignLeading, fyne.TextStyle{Bold: true, Italic: true}),
		layout.NewSpacer(),
		widget.NewLabel("Extraction Mode:"),
		mw.modeSelect,
		mw.autoCopyCheck,
		widget.NewSeparator(),
		browseFilesBtn,
		browseFolderBtn,
	)

	// 2. Center Content (Split Top/Bottom)
	
	// Top part of the split: Dropzone and Preview
	mw.dropZone = NewDropZone("DRAG & DROP PNG/JSON HERE", func(paths []string) {
		mw.loadFiles(paths)
	})

	mw.previewImg = canvas.NewImageFromImage(nil)
	mw.previewImg.FillMode = canvas.ImageFillContain
	
	// previewBox ensures the right side of the split has a stable size and doesn't jump
	previewBoxCont := container.NewGridWrap(fyne.NewSize(300, 300), mw.previewImg)

	mw.previewLabel = widget.NewLabel("")
	mw.previewLabel.Alignment = fyne.TextAlignCenter
	mw.previewLabel.TextStyle = fyne.TextStyle{Monospace: true}

	mw.previewCont = container.NewBorder(nil, mw.previewLabel, nil, nil, previewBoxCont)
	mw.previewCont.Hide()

	// Use an HSplit for DropZone | Preview
	topSplit := container.NewHSplit(
		mw.dropZone,
		mw.previewCont,
	)
	topSplit.Offset = 0.6 // Initial balance

	// Bottom part of the split: Results
	mw.promptEntry = NewReadOnlyEntry()
	mw.promptEntry.Wrapping = fyne.TextWrapWord
	mw.promptEntry.TextStyle = fyne.TextStyle{Monospace: true}

	mw.summaryEntry = NewReadOnlyEntry()
	mw.summaryEntry.Wrapping = fyne.TextWrapWord

	tabs := container.NewAppTabs(
		container.NewTabItemWithIcon("Extracted Prompts", theme.FileTextIcon(), container.NewScroll(mw.promptEntry)),
		container.NewTabItemWithIcon("Summary", theme.InfoIcon(), container.NewScroll(mw.summaryEntry)),
	)

	// MAIN VSplit (Top Area vs Results)
	mainSplit := container.NewVSplit(
		topSplit,
		tabs,
	)
	mainSplit.Offset = 0.4 // 40% top, 60% bottom

	// 3. Footer (Progress + Actions + Status)
	mw.progressBar = widget.NewProgressBarInfinite()
	mw.progressBar.Hide()

	mw.copyAllBtn = widget.NewButtonWithIcon("Copy All", theme.ContentCopyIcon(), mw.copyAllPrompts)
	mw.copyFirstBtn = widget.NewButtonWithIcon("Copy First", theme.ContentCopyIcon(), mw.copyFirstPrompt)
	mw.saveBtn = widget.NewButtonWithIcon("Save To File", theme.DocumentSaveIcon(), mw.saveToFile)
	mw.clearBtn = widget.NewButtonWithIcon("Clear Results", theme.DeleteIcon(), mw.clearResults)

	actions := container.NewHBox(
		layout.NewSpacer(),
		mw.copyAllBtn,
		mw.copyFirstBtn,
		mw.saveBtn,
		mw.clearBtn,
		layout.NewSpacer(),
	)

	mw.statusLabel = widget.NewLabel("Ready")
	mw.statusLabel.TextStyle = fyne.TextStyle{Italic: true}

	footer := container.NewVBox(
		mw.progressBar,
		container.NewPadded(actions),
		container.NewHBox(layout.NewSpacer(), mw.statusLabel),
	)

	// Set window level drop as well
	mw.window.SetOnDropped(func(p fyne.Position, uris []fyne.URI) {
		var paths []string
		for _, u := range uris {
			if u.Scheme() == "file" {
				paths = append(paths, u.Path())
			}
		}
		if len(paths) > 0 {
			mw.dropZone.Flash()
			mw.loadFiles(paths)
		}
	})

	// Final Layout
	mw.window.SetContent(container.NewBorder(
		container.NewPadded(header),
		footer,
		nil,
		nil,
		mainSplit,
	))
	
	mw.updateButtonStates()
}

func (mw *MainWindow) setupShortcuts() {
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
}

func (mw *MainWindow) setupMenu() {
	fileMenu := fyne.NewMenu("File",
		fyne.NewMenuItem("Open File(s)...", mw.browseFiles),
		fyne.NewMenuItem("Open Folder...", mw.browseFolder),
		fyne.NewMenuItemSeparator(),
		fyne.NewMenuItem("Save to File...", mw.saveToFile),
		fyne.NewMenuItemSeparator(),
		fyne.NewMenuItem("Exit", func() { mw.app.Quit() }),
	)

	editMenu := fyne.NewMenu("Edit",
		fyne.NewMenuItem("Copy All Prompts", mw.copyAllPrompts),
		fyne.NewMenuItem("Copy First Prompt", mw.copyFirstPrompt),
		fyne.NewMenuItem("Clear Results", mw.clearResults),
	)

	helpMenu := fyne.NewMenu("Help",
		fyne.NewMenuItem("About", func() {
			showAbout(mw.window)
		}),
	)

	mw.window.SetMainMenu(fyne.NewMainMenu(fileMenu, editMenu, helpMenu))
}

func (mw *MainWindow) toggleMode() {
	if mw.state.mode == "ComfyUI" {
		mw.modeSelect.SetSelected("Parameters")
	} else {
		mw.modeSelect.SetSelected("ComfyUI")
	}
}

func (mw *MainWindow) browseFiles() {
	d := dialog.NewFileOpen(func(r fyne.URIReadCloser, err error) {
		if err != nil || r == nil {
			return
		}
		mw.loadFiles([]string{r.URI().Path()})
	}, mw.window)
	d.Show()
}

func (mw *MainWindow) browseFolder() {
	d := dialog.NewFolderOpen(func(lu fyne.ListableURI, err error) {
		if err != nil || lu == nil {
			return
		}
		mw.loadFiles([]string{lu.Path()})
	}, mw.window)
	d.Show()
}

func (mw *MainWindow) loadFiles(paths []string) {
	if mw.state.busy {
		dialog.ShowInformation("Busy", "Extraction already in progress. Please wait.", mw.window)
		return
	}

	var valid []string
	for _, p := range paths {
		info, err := os.Stat(p)
		if err != nil {
			mw.statusLabel.SetText(fmt.Sprintf("Error reading file: %v", err))
			continue
		}
		if info.IsDir() {
			filepath.WalkDir(p, func(path string, d fs.DirEntry, err error) error {
				if err != nil {
					return nil
				}
				if !d.IsDir() {
					ext := strings.ToLower(filepath.Ext(path))
					if ext == ".png" || ext == ".json" {
						valid = append(valid, path)
					}
				}
				return nil
			})
		} else {
			ext := strings.ToLower(filepath.Ext(p))
			if ext == ".png" || ext == ".json" {
				valid = append(valid, p)
			}
		}
	}

	if len(valid) == 0 {
		dialog.ShowInformation("Warning", "No valid PNG or JSON files found", mw.window)
		return
	}

	mw.processFiles(valid)
}

func (mw *MainWindow) setUIBusy(busy bool) {
	mw.state.busy = busy
	if busy {
		mw.progressBar.Show()
		mw.statusLabel.SetText("Processing...")
	} else {
		mw.progressBar.Hide()
	}
	mw.updateButtonStates()
}

func (mw *MainWindow) processFiles(files []string) {
	if mw.state.busy && len(mw.state.currentFiles) > 0 {
		return
	}
	mw.setUIBusy(true)
	mw.state.currentFiles = files

	// Immediately clear old state and hide preview
	mw.promptEntry.SetText("")
	mw.summaryEntry.SetText("")
	mw.previewCont.Hide()

	go func() {
		// Load thumbnail off-thread
		var thumbImg image.Image
		var thumbW, thumbH int
		if len(files) == 1 && strings.ToLower(filepath.Ext(files[0])) == ".png" {
			img, w, h, err := loadThumbnail(files[0], 400)
			if err == nil {
				thumbImg = img
				thumbW, thumbH = w, h
			}
		}

		// Run extraction
		results := make([]*extractor.ExtractionResult, 0, len(files))
		e := &extractor.PromptExtractor{}
		for _, f := range files {
			var r *extractor.ExtractionResult
			var err error
			ext := strings.ToLower(filepath.Ext(f))
			switch ext {
			case ".json":
				r, err = e.ExtractJSON(f)
			case ".png":
				if mw.state.mode == "ComfyUI" {
					r, err = e.ExtractComfyUI(f)
				} else {
					r, err = e.ExtractParameters(f)
				}
			}

			if err != nil {
				r = &extractor.ExtractionResult{
					FileInfo: extractor.FileInfo{Filename: filepath.Base(f)},
					Error:    err.Error(),
				}
			}
			results = append(results, r)
		}

		// Post all UI updates back on the main thread
		fyne.Do(func() {
			if thumbImg != nil {
				mw.previewImg.Image = thumbImg
				mw.previewImg.Refresh()
				mw.previewLabel.SetText(fmt.Sprintf("%d×%d | %s", thumbW, thumbH, filepath.Base(files[0])))
				mw.previewCont.Show()
			}
			mw.onExtractionFinished(results)
		})
	}()
}

func (mw *MainWindow) onExtractionFinished(results []*extractor.ExtractionResult) {
	var promptLines []string
	var summaryLines []string
	var allTexts []string

	totalPrompts := 0
	filesWithPrompts := 0

	for i, result := range results {
		prompts := result.PositivePrompts
		if len(prompts) == 0 {
			if result.Error != "" {
				summaryLines = append(summaryLines, fmt.Sprintf("Error in %s: %s", result.FileInfo.Filename, result.Error))
			}
			continue
		}
		filesWithPrompts++
		totalPrompts += len(prompts)

		if len(results) > 1 {
			promptLines = append(promptLines,
				fmt.Sprintf("=== %s [%s] ===", result.FileInfo.Filename, result.ExtractionMethod))
		}

		for j, p := range prompts {
			if len(prompts) > 1 {
				promptLines = append(promptLines, fmt.Sprintf("\nPrompt %d - %s:", j+1, p.Title))
				promptLines = append(promptLines, strings.Repeat("-", 40))
			}
			promptLines = append(promptLines, p.Text)
			allTexts = append(allTexts, p.Text)
			if j < len(prompts)-1 {
				promptLines = append(promptLines, "")
			}
		}
		if i < len(results)-1 {
			promptLines = append(promptLines, "\n"+strings.Repeat("=", 60)+"\n")
		}

		summaryLines = append(summaryLines, fmt.Sprintf("File: %s", result.FileInfo.Filename))
		summaryLines = append(summaryLines, fmt.Sprintf("  Method: %s", result.ExtractionMethod))
		summaryLines = append(summaryLines, fmt.Sprintf("  Prompts found: %d", len(prompts)))
		summaryLines = append(summaryLines, "")
	}

	summaryHeader := []string{
		"EXTRACTION SUMMARY",
		strings.Repeat("-", 20),
		fmt.Sprintf("Total files processed: %d", len(results)),
		fmt.Sprintf("Files with prompts:    %d", filesWithPrompts),
		fmt.Sprintf("Total prompts found:   %d", totalPrompts),
		strings.Repeat("-", 20),
		"",
	}
	summaryLines = append(summaryHeader, summaryLines...)

	mw.state.currentResults = results
	mw.state.allPromptTexts = allTexts

	// Update UI on main thread
	mw.promptEntry.SetText(strings.Join(promptLines, "\n"))
	mw.summaryEntry.SetText(strings.Join(summaryLines, "\n"))
	mw.statusLabel.SetText(fmt.Sprintf("Found %d prompts in %d files", totalPrompts, filesWithPrompts))
	
	if mw.state.autoCopy && len(allTexts) > 0 {
		mw.copyAllPrompts()
		mw.statusLabel.SetText(mw.statusLabel.Text + " [COPIED TO CLIPBOARD]")
	}
	
	mw.setUIBusy(false)
}

func (mw *MainWindow) copyAllPrompts() {
	if len(mw.state.allPromptTexts) == 0 {
		return
	}
	text := strings.Join(mw.state.allPromptTexts, "\n\n")
	clipboard.WriteAll(text)
}

func (mw *MainWindow) copyFirstPrompt() {
	if len(mw.state.allPromptTexts) == 0 {
		return
	}
	clipboard.WriteAll(mw.state.allPromptTexts[0])
}

func (mw *MainWindow) clearResults() {
	mw.state.currentFiles = nil
	mw.state.currentResults = nil
	mw.state.allPromptTexts = nil
	mw.promptEntry.SetText("")
	mw.summaryEntry.SetText("")
	mw.previewCont.Hide()
	mw.statusLabel.SetText("Ready")
	mw.updateButtonStates()
}

func (mw *MainWindow) updateButtonStates() {
	hasResults := len(mw.state.allPromptTexts) > 0 && !mw.state.busy
	if hasResults {
		mw.copyAllBtn.Enable()
		mw.copyFirstBtn.Enable()
		mw.saveBtn.Enable()
	} else {
		mw.copyAllBtn.Disable()
		mw.copyFirstBtn.Disable()
		mw.saveBtn.Disable()
	}

	if mw.state.busy || (len(mw.state.currentFiles) == 0) {
		mw.clearBtn.Disable()
	} else {
		mw.clearBtn.Enable()
	}
}

func (mw *MainWindow) saveToFile() {
	if len(mw.state.allPromptTexts) == 0 {
		return
	}

	defaultName := "extracted_prompts.txt"
	if len(mw.state.currentFiles) == 1 {
		base := strings.TrimSuffix(filepath.Base(mw.state.currentFiles[0]),
			filepath.Ext(mw.state.currentFiles[0]))
		defaultName = base + "_prompts.txt"
	}

	d := dialog.NewFileSave(func(w fyne.URIWriteCloser, err error) {
		if w == nil || err != nil {
			return
		}
		defer w.Close()

		writer := bufio.NewWriter(w)
		fmt.Fprintln(writer, strings.Repeat("=", 60))
		fmt.Fprintln(writer, "COMFYUI POSITIVE PROMPTS EXTRACTION")
		fmt.Fprintln(writer, strings.Repeat("=", 60))
		fmt.Fprintf(writer, "\nExtractor mode: %s\n", mw.state.mode)
		fmt.Fprintf(writer, "Files processed: %d\n", len(mw.state.currentFiles))
		fmt.Fprintf(writer, "Total prompts: %d\n", len(mw.state.allPromptTexts))
		fmt.Fprintf(writer, "Extraction date: %s\n", time.Now().Format("2006-01-02 15:04:05"))
		fmt.Fprintln(writer, "\n"+strings.Repeat("=", 60))

		promptIdx := 0
		for i, result := range mw.state.currentResults {
			if len(result.PositivePrompts) == 0 {
				continue
			}
			if len(mw.state.currentResults) > 1 {
				fmt.Fprintf(writer, "FILE: %s\nMethod: %s\n", result.FileInfo.Filename, result.ExtractionMethod)
				fmt.Fprintln(writer, strings.Repeat("-", 60))
			}
			for j, p := range result.PositivePrompts {
				if len(result.PositivePrompts) > 1 {
					fmt.Fprintf(writer, "Prompt %d - %s:\n%s\n", j+1, p.Title, strings.Repeat("-", 40))
				}
				if promptIdx < len(mw.state.allPromptTexts) {
					fmt.Fprintln(writer, mw.state.allPromptTexts[promptIdx])
					promptIdx++
				}
			}
			if i < len(mw.state.currentResults)-1 {
				fmt.Fprintln(writer, "\n"+strings.Repeat("=", 60))
			}
		}
		writer.Flush()
	}, mw.window)
	d.SetFileName(defaultName)
	d.Show()
}

func (mw *MainWindow) ShowAndRun() {
	mw.window.ShowAndRun()
}

func loadThumbnail(filePath string, maxSize int) (image.Image, int, int, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return nil, 0, 0, err
	}
	defer f.Close()

	img, _, err := image.Decode(f)
	if err != nil {
		return nil, 0, 0, err
	}

	origW := img.Bounds().Dx()
	origH := img.Bounds().Dy()

	// Scale down maintaining aspect ratio
	scale := float64(maxSize) / math.Max(float64(origW), float64(origH))
	if scale >= 1.0 {
		return img, origW, origH, nil
	}
	newW := int(float64(origW) * scale)
	newH := int(float64(origH) * scale)
	dst := image.NewRGBA(image.Rect(0, 0, newW, newH))
	draw.BiLinear.Scale(dst, dst.Bounds(), img, img.Bounds(), draw.Over, nil)
	return dst, origW, origH, nil
}

