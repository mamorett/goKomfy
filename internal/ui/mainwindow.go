package ui

import (
	"bufio"
	"context"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io/fs"
	"log"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
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
	mu              sync.Mutex
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
	mw.dropZone = NewDropZone("DRAG & DROP PNG/JSON HERE")

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
		defer r.Close()
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

func (mw *MainWindow) isBusy() bool {
	mw.state.mu.Lock()
	defer mw.state.mu.Unlock()
	return mw.state.busy
}

func (mw *MainWindow) loadFiles(paths []string) {
	// If busy, we simply ignore new drops instead of showing a blocking dialog.
	// This prevents the "double-drop" or rapid drop from freezing the app with dialogs.
	if mw.isBusy() {
		mw.statusLabel.SetText("Processing in progress — please wait...")
		log.Printf("[DEBUG] Drop rejected because app is busy")
		return
	}
	log.Printf("[DEBUG] Processing %d files", len(paths))

	var valid []string
	for _, p := range paths {
		info, err := os.Stat(p)
		if err != nil {
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
		return
	}

	mw.processFiles(valid)
}

func (mw *MainWindow) setUIBusy(busy bool) {
	mw.state.mu.Lock()
	mw.state.busy = busy
	mw.state.mu.Unlock()

	if busy {
		mw.progressBar.Show()
		mw.statusLabel.SetText("Processing...")
	} else {
		mw.progressBar.Hide()
	}
	mw.updateButtonStates()
}

func (mw *MainWindow) processFiles(files []string) {
	if mw.isBusy() {
		return
	}
	mw.setUIBusy(true)
	mw.state.currentFiles = files

	// Release previous preview image reference (3.4)
	mw.previewImg.Image = nil
	mw.previewImg.Refresh()

	// Immediately clear old state and hide preview
	mw.promptEntry.SetText("")
	mw.summaryEntry.SetText("")
	mw.previewCont.Hide()

	// 1.2. Capture mode before spawning the processing goroutine
	currentMode := mw.state.mode

	// 2.2. Create context for cancellation (120s total)
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)

	// 2.1. Add a timeout-based safety net for the busy flag
	go func() {
		time.Sleep(130 * time.Second)
		if mw.isBusy() {
			log.Printf("[WARN] Busy flag safety net triggered after 130s")
			fyne.Do(func() {
				mw.setUIBusy(false)
				mw.statusLabel.SetText("Processing timed out - recovered")
			})
		}
	}()

	go func() {
		defer cancel()
		finished := false
		defer func() {
			if !finished {
				if r := recover(); r != nil {
					log.Printf("[PANIC] %v", r)
					done := make(chan struct{})
					go func() {
						fyne.Do(func() {
							mw.setUIBusy(false)
							dialog.ShowError(fmt.Errorf("internal panic: %v", r), mw.window)
							close(done)
						})
					}()
					select {
					case <-done:
					case <-time.After(2 * time.Second):
						log.Printf("[ERROR] fyne.Do timed out in panic recovery, forcing busy=false")
						mw.state.mu.Lock()
						mw.state.busy = false
						mw.state.mu.Unlock()
					}
				}
			}
		}()

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
			if ctx.Err() != nil {
				log.Printf("[DEBUG] Processing timed out/cancelled")
				break
			}

			var r *extractor.ExtractionResult
			var err error
			ext := strings.ToLower(filepath.Ext(f))

			// 4.3 Validate PNG files before full processing (Check size)
			if ext == ".png" {
				info, err := os.Stat(f)
				if err == nil && info.Size() > 200*1024*1024 { // 200MB limit
					r = &extractor.ExtractionResult{
						FileInfo: extractor.FileInfo{Filename: filepath.Base(f)},
						Error:    "File too large (> 200MB)",
					}
					results = append(results, r)
					continue
				}
			}

			// 4.2. Per-file timeout
			type extractRes struct {
				r   *extractor.ExtractionResult
				err error
			}
			fileChan := make(chan extractRes, 1)

			go func(file string, currentExt string) {
				var r *extractor.ExtractionResult
				var err error
				switch currentExt {
				case ".json":
					r, err = e.ExtractJSON(file)
				case ".png":
					if currentMode == "ComfyUI" {
						r, err = e.ExtractComfyUI(file)
					} else {
						r, err = e.ExtractParameters(file)
					}
				}
				fileChan <- extractRes{r, err}
			}(f, ext)

			select {
			case res := <-fileChan:
				r = res.r
				err = res.err
			case <-time.After(30 * time.Second):
				log.Printf("[ERROR] Extraction timed out for file: %s", f)
				err = fmt.Errorf("extraction timed out after 30s")
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
				ratioStr := calculateAspectRatio(thumbW, thumbH)
				mw.previewLabel.SetText(fmt.Sprintf("[%s] %d×%d | %s", ratioStr, thumbW, thumbH, filepath.Base(files[0])))
				mw.previewCont.Show()
			}
			mw.onExtractionFinished(results)
			finished = true
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
	mw.previewImg.Image = nil
	mw.previewImg.Refresh()
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

	// 3.3. Use image.DecodeConfig before full decode
	config, _, err := image.DecodeConfig(f)
	if err != nil {
		return nil, 0, 0, err
	}

	// Reset file pointer for full decode
	_, err = f.Seek(0, 0)
	if err != nil {
		return nil, 0, 0, err
	}

	// Dimension guardrail
	if config.Width > 8192 || config.Height > 8192 {
		return nil, config.Width, config.Height, fmt.Errorf("image too large (%dx%d)", config.Width, config.Height)
	}

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

func calculateAspectRatio(w, h int) string {
	if h == 0 {
		return "0:0"
	}
	ratio := float64(w) / float64(h)

	type commonRatio struct {
		label string
		val   float64
	}

	ratios := []commonRatio{
		{"1:1", 1.0},
		{"4:3", 4.0 / 3.0},
		{"3:4", 3.0 / 4.0},
		{"3:2", 3.0 / 2.0},
		{"2:3", 2.0 / 3.0},
		{"16:9", 16.0 / 9.0},
		{"9:16", 9.0 / 16.0},
		{"9:7", 9.0 / 7.0},
		{"7:9", 7.0 / 9.0},
	}

	bestMatch := ""
	minDiff := 10.0 // Large initial value

	for _, r := range ratios {
		diff := math.Abs(ratio - r.val)
		if diff < minDiff {
			minDiff = diff
			bestMatch = r.label
		}
	}

	// If the difference is too large (e.g., > 0.05), just return the simplified fraction or decimal?
	// But the user asked to round to meaningful ones. If it's very far, maybe we just use the closest.
	// 0.05 is a reasonable threshold for "meaningful" matching.
	if minDiff > 0.05 {
		// Fallback to a simple GCD if it doesn't match common ones well
		g := gcd(w, h)
		return fmt.Sprintf("%d:%d", w/g, h/g)
	}

	return bestMatch
}

func gcd(a, b int) int {
	for b != 0 {
		a, b = b, a%b
	}
	return a
}

