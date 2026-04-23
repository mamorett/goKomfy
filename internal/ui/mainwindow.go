package ui

import (
	"bufio"
	"context"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
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

const (
	maxThumbnailSize = 400 * 400 * 4 // RGBA bytes
)

type AppState struct {
	mu            sync.Mutex
	currentFile   string
	currentResult *extractor.ExtractionResult
	promptTexts   []string
	mode          string // "ComfyUI" or "Parameters"
	busy          bool
	autoCopy      bool
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
	previewBoxCont *fyne.Container

	promptScroll  *container.Scroll
	summaryScroll *container.Scroll

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
		// Re-extract if we already have a file loaded
		if mw.state.currentFile != "" && !mw.state.busy {
			mw.processFile(mw.state.currentFile)
		}
	})
	mw.modeSelect.SetSelected(mw.state.mode)

	mw.autoCopyCheck = widget.NewCheck("Auto-copy to Clipboard", func(b bool) {
		mw.state.autoCopy = b
	})

	browseFilesBtn := widget.NewButtonWithIcon("Open File", theme.FileIcon(), func() {
		mw.browseFiles()
	})

	header := container.NewHBox(
		widget.NewLabelWithStyle("goKomfy", fyne.TextAlignLeading, fyne.TextStyle{Bold: true, Italic: true}),
		layout.NewSpacer(),
		widget.NewLabel("Extraction Mode:"),
		mw.modeSelect,
		mw.autoCopyCheck,
		widget.NewSeparator(),
		browseFilesBtn,
	)

	// 2. Center Content (Split Top/Bottom)

	// Top part of the split: Dropzone and Preview
	mw.dropZone = NewDropZone("DRAG & DROP PNG/JSON HERE")

	mw.previewImg = canvas.NewImageFromImage(nil)
	mw.previewImg.FillMode = canvas.ImageFillContain

	// previewBox ensures the right side of the split has a stable size and doesn't jump
	mw.previewBoxCont = container.NewGridWrap(fyne.NewSize(300, 300), mw.previewImg)

	mw.previewLabel = widget.NewLabel("")
	mw.previewLabel.Alignment = fyne.TextAlignCenter
	mw.previewLabel.TextStyle = fyne.TextStyle{Monospace: true}

	mw.previewCont = container.NewBorder(nil, mw.previewLabel, nil, nil, mw.previewBoxCont)
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

	mw.promptScroll = container.NewScroll(mw.promptEntry)
	mw.summaryScroll = container.NewScroll(mw.summaryEntry)

	tabs := container.NewAppTabs(
		container.NewTabItemWithIcon("Extracted Prompts", theme.FileTextIcon(), mw.promptScroll),
		container.NewTabItemWithIcon("Summary", theme.InfoIcon(), mw.summaryScroll),
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
		if len(uris) > 0 && uris[0].Scheme() == "file" {
			mw.dropZone.Flash()
			mw.loadFile(uris[0].Path())
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
		fyne.NewMenuItem("Open File...", mw.browseFiles),
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
		mw.loadFile(r.URI().Path())
	}, mw.window)
	d.Show()
}

func (mw *MainWindow) isBusy() bool {
	mw.state.mu.Lock()
	defer mw.state.mu.Unlock()
	return mw.state.busy
}

func (mw *MainWindow) loadFile(path string) {
	if mw.isBusy() {
		mw.statusLabel.SetText("Processing in progress — please wait...")
		return
	}

	ext := strings.ToLower(filepath.Ext(path))
	if ext != ".png" && ext != ".json" {
		return
	}

	mw.processFile(path)
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

func (mw *MainWindow) processFile(file string) {
	if mw.isBusy() {
		return
	}
	mw.setUIBusy(true)
	mw.state.currentFile = file

	// Release previous preview image reference
	mw.previewImg.Image = nil
	mw.previewImg.Refresh()

	// Immediately clear old state and hide preview
	mw.promptEntry.SetText("")
	mw.summaryEntry.SetText("")
	mw.previewCont.Hide()

	currentMode := mw.state.mode
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)

	// Add a timeout-based safety net for the busy flag
	go func() {
		select {
		case <-ctx.Done():
		case <-time.After(130 * time.Second):
			if mw.isBusy() {
				log.Printf("[WARN] Busy flag safety net triggered after 130s")
				fyne.Do(func() {
					mw.setUIBusy(false)
					mw.statusLabel.SetText("Processing timed out - recovered")
				})
			}
		}
	}()

	go func() {
		defer cancel()
		finished := false
		defer func() {
			if !finished {
				if r := recover(); r != nil {
					log.Printf("[PANIC] %v", r)
					fyne.Do(func() {
						mw.setUIBusy(false)
						dialog.ShowError(fmt.Errorf("internal panic: %v", r), mw.window)
					})
				}
			}
		}()

		var thumbImg image.Image
		var thumbW, thumbH int
		if strings.ToLower(filepath.Ext(file)) == ".png" {
			select {
			case <-ctx.Done():
				log.Printf("[DEBUG] Thumbnail loading cancelled")
			case res := <-getThumbnailAsync(ctx, file, 400):
				thumbImg, thumbW, thumbH = res.img, res.w, res.h
			}
		}

		// Run extraction
		e := &extractor.PromptExtractor{}
		var result *extractor.ExtractionResult
		var err error
		ext := strings.ToLower(filepath.Ext(file))

		// Validate PNG files before full processing (Check size)
		if ext == ".png" {
			info, errStat := os.Stat(file)
			if errStat == nil && info.Size() > 200*1024*1024 { // 200MB limit
				result = &extractor.ExtractionResult{
					FileInfo: extractor.FileInfo{Filename: filepath.Base(file)},
					Error:    "File too large (> 200MB)",
				}
			}
		}

		if result == nil {
			var opts *extractor.ExtractionOptions
			if thumbImg != nil {
				opts = &extractor.ExtractionOptions{Width: thumbW, Height: thumbH}
			}

			switch ext {
			case ".json":
				result, err = e.ExtractJSON(file)
			case ".png":
				if currentMode == "ComfyUI" {
					result, err = e.ExtractComfyUI(file, opts)
				} else {
					result, err = e.ExtractParameters(file, opts)
				}
			}

			if err != nil {
				result = &extractor.ExtractionResult{
					FileInfo: extractor.FileInfo{Filename: filepath.Base(file)},
					Error:    err.Error(),
				}
			}
		}

		// Post all UI updates back on the main thread
		fyne.Do(func() {
			if thumbImg != nil {
				// Create a fresh canvas.Image to force Fyne to release old textures
				newImg := canvas.NewImageFromImage(thumbImg)
				newImg.FillMode = canvas.ImageFillContain

				// Replace the image in the preview container
				mw.previewBoxCont.Objects[0] = newImg
				mw.previewBoxCont.Refresh()
				mw.previewImg = newImg // Update the reference

				ratioStr := calculateAspectRatio(thumbW, thumbH)
				mw.previewLabel.SetText(fmt.Sprintf("[%s] %d×%d | %s", ratioStr, thumbW, thumbH, filepath.Base(file)))
				mw.previewCont.Show()
			}
			mw.onExtractionFinished(result)
			finished = true
		})
	}()
}

func (mw *MainWindow) onExtractionFinished(result *extractor.ExtractionResult) {
	var promptLines []string
	var summaryLines []string
	var allTexts []string

	prompts := result.PositivePrompts
	if len(prompts) == 0 {
		if result.Error != "" {
			summaryLines = append(summaryLines, fmt.Sprintf("Error in %s: %s", result.FileInfo.Filename, result.Error))
		} else {
			summaryLines = append(summaryLines, fmt.Sprintf("No prompts found in %s", result.FileInfo.Filename))
		}
	} else {
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

		summaryLines = append(summaryLines, fmt.Sprintf("File: %s", result.FileInfo.Filename))
		summaryLines = append(summaryLines, fmt.Sprintf("Method: %s", result.ExtractionMethod))
		summaryLines = append(summaryLines, fmt.Sprintf("Prompts found: %d", len(prompts)))
	}

	summaryHeader := []string{
		"EXTRACTION SUMMARY",
		strings.Repeat("-", 20),
		"",
	}
	summaryLines = append(summaryHeader, summaryLines...)

	mw.state.currentResult = result
	mw.state.promptTexts = allTexts

	// Update UI on main thread
	mw.promptEntry.SetText(strings.Join(promptLines, "\n"))
	mw.summaryEntry.SetText(strings.Join(summaryLines, "\n"))
	mw.statusLabel.SetText(fmt.Sprintf("Found %d prompts in %s", len(allTexts), result.FileInfo.Filename))

	if mw.state.autoCopy && len(allTexts) > 0 {
		mw.copyAllPrompts()
		mw.statusLabel.SetText(mw.statusLabel.Text + " [COPIED TO CLIPBOARD]")
	}

	mw.setUIBusy(false)
}

func (mw *MainWindow) copyAllPrompts() {
	if len(mw.state.promptTexts) == 0 {
		return
	}
	text := strings.Join(mw.state.promptTexts, "\n\n")
	clipboard.WriteAll(text)
}

func (mw *MainWindow) copyFirstPrompt() {
	if len(mw.state.promptTexts) == 0 {
		return
	}
	clipboard.WriteAll(mw.state.promptTexts[0])
}

func (mw *MainWindow) clearResults() {
	mw.state.mu.Lock()
	mw.state.currentFile = ""
	mw.state.currentResult = nil
	mw.state.promptTexts = nil
	mw.state.mu.Unlock()

	// Recreate text widgets to fully release internal state
	mw.promptEntry = NewReadOnlyEntry()
	mw.promptEntry.Wrapping = fyne.TextWrapWord
	mw.promptEntry.TextStyle = fyne.TextStyle{Monospace: true}

	mw.summaryEntry = NewReadOnlyEntry()
	mw.summaryEntry.Wrapping = fyne.TextWrapWord

	// Update the scroll containers
	mw.promptScroll.Content = mw.promptEntry
	mw.promptScroll.Refresh()
	mw.summaryScroll.Content = mw.summaryEntry
	mw.summaryScroll.Refresh()

	// Reset preview
	newImg := canvas.NewImageFromImage(nil)
	newImg.FillMode = canvas.ImageFillContain
	mw.previewBoxCont.Objects[0] = newImg
	mw.previewBoxCont.Refresh()
	mw.previewImg = newImg
	mw.previewCont.Hide()

	mw.statusLabel.SetText("Ready")
	mw.updateButtonStates()
}

func (mw *MainWindow) updateButtonStates() {
	hasResults := len(mw.state.promptTexts) > 0 && !mw.state.busy
	if hasResults {
		mw.copyAllBtn.Enable()
		mw.copyFirstBtn.Enable()
		mw.saveBtn.Enable()
	} else {
		mw.copyAllBtn.Disable()
		mw.copyFirstBtn.Disable()
		mw.saveBtn.Disable()
	}

	if mw.state.busy || (mw.state.currentFile == "") {
		mw.clearBtn.Disable()
	} else {
		mw.clearBtn.Enable()
	}
}

func (mw *MainWindow) saveToFile() {
	if len(mw.state.promptTexts) == 0 {
		return
	}

	base := strings.TrimSuffix(filepath.Base(mw.state.currentFile),
		filepath.Ext(mw.state.currentFile))
	defaultName := base + "_prompts.txt"

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
		fmt.Fprintf(writer, "File processed: %s\n", mw.state.currentFile)
		fmt.Fprintf(writer, "Total prompts: %d\n", len(mw.state.promptTexts))
		fmt.Fprintf(writer, "Extraction date: %s\n", time.Now().Format("2006-01-02 15:04:05"))
		fmt.Fprintln(writer, "\n"+strings.Repeat("=", 60))

		result := mw.state.currentResult
		for j, p := range result.PositivePrompts {
			if len(result.PositivePrompts) > 1 {
				fmt.Fprintf(writer, "Prompt %d - %s:\n%s\n", j+1, p.Title, strings.Repeat("-", 40))
			}
			fmt.Fprintln(writer, p.Text)
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

	// Explicitly release original to free memory faster
	img = nil

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

type thumbnailResult struct {
	img image.Image
	w   int
	h   int
}

func getThumbnailAsync(ctx context.Context, filePath string, maxSize int) <-chan thumbnailResult {
	ch := make(chan thumbnailResult, 1)
	go func() {
		defer close(ch)
		img, w, h, err := loadThumbnail(filePath, maxSize)
		if err == nil {
			select {
			case ch <- thumbnailResult{img, w, h}:
			case <-ctx.Done():
			}
		}
	}()
	return ch
}

