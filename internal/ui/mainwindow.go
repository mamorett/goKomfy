package ui

import (
	"bufio"
	"context"
	"fmt"
	"image"
	"image/color"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
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
	cancel        context.CancelFunc
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

	dropZone       *DropZone
	previewImg     *canvas.Image
	previewLabel   *widget.Label
	previewCardBg  *canvas.Rectangle
	previewCont    *fyne.Container
	previewBoxCont *fyne.Container

	promptScroll  *container.Scroll
	summaryScroll *container.Scroll
	tabs          *container.AppTabs
	resultsStack  *fyne.Container
	emptyState    *fyne.Container

	copyBtn     *widget.Button
	saveBtn     *widget.Button
	clearBtn    *widget.Button
	aboutBtn    *widget.Button
	statusLabel *widget.Label
	statusDot   *canvas.Circle
}

func NewMainWindow(a fyne.App) *MainWindow {
	a.Settings().SetTheme(NewKomfyTheme())
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
		mw.setMode(s)
		curFile := mw.getCurrentFile()
		if curFile != "" && !mw.isBusy() {
			mw.processFile(curFile)
		}
	})
	mw.modeSelect.SetSelected(mw.getMode())

	mw.autoCopyCheck = widget.NewCheck("Auto-copy", func(b bool) {
		mw.setAutoCopy(b)
	})

	browseFilesBtn := widget.NewButtonWithIcon("Open File", theme.FileIcon(), func() {
		mw.browseFiles()
	})
	browseFilesBtn.Importance = widget.LowImportance

	header := container.NewHBox(
		widget.NewIcon(resourceLogoPng),
		widget.NewLabelWithStyle("goKomfy", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}),
		layout.NewSpacer(),
		container.NewVBox(layout.NewSpacer(), widget.NewLabel("Mode:"), layout.NewSpacer()),
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

	mw.previewCardBg = canvas.NewRectangle(color.RGBA{R: 0x3b, G: 0x42, B: 0x52, A: 0xff}) // nord1
	mw.previewCardBg.CornerRadius = 8

	mw.previewBoxCont = container.NewStack(mw.previewCardBg, mw.previewImg)

	mw.previewLabel = widget.NewLabel("")
	mw.previewLabel.Alignment = fyne.TextAlignCenter
	mw.previewLabel.TextStyle = fyne.TextStyle{Monospace: true}
	mw.previewLabel.Truncation = fyne.TextTruncateEllipsis

	mw.previewCont = container.NewBorder(nil, mw.previewLabel, nil, nil, mw.previewBoxCont)

	// Use an HSplit for DropZone | Preview
	topSplit := container.NewHSplit(
		mw.dropZone,
		mw.previewCont,
	)
	topSplit.Offset = 0.5

	// Bottom part of the split: Results
	mw.promptEntry = NewReadOnlyEntry()
	mw.promptEntry.Wrapping = fyne.TextWrapWord
	mw.promptEntry.TextStyle = fyne.TextStyle{Monospace: true}

	mw.summaryEntry = NewReadOnlyEntry()
	mw.summaryEntry.Wrapping = fyne.TextWrapWord

	mw.promptScroll = container.NewScroll(mw.promptEntry)
	mw.summaryScroll = container.NewScroll(mw.summaryEntry)

	mw.tabs = container.NewAppTabs(
		container.NewTabItemWithIcon("Extracted Prompts", theme.FileTextIcon(), mw.promptScroll),
		container.NewTabItemWithIcon("Summary", theme.InfoIcon(), mw.summaryScroll),
	)

	// Empty State
	emptyIcon := widget.NewIcon(theme.InfoIcon())
	emptyLabel := widget.NewLabelWithStyle("No file loaded.\nDrop a PNG or JSON file to begin.", fyne.TextAlignCenter, fyne.TextStyle{Italic: true})
	mw.emptyState = container.NewCenter(container.NewVBox(
		container.NewCenter(emptyIcon),
		emptyLabel,
	))

	mw.resultsStack = container.NewStack(mw.tabs, mw.emptyState)
	mw.tabs.Hide() // Hide tabs by default

	// MAIN VSplit (Top Area vs Results)
	mainSplit := container.NewVSplit(
		topSplit,
		mw.resultsStack,
	)
	mainSplit.Offset = 0.45

	// 3. Footer (Progress + Actions + Status)
	mw.progressBar = widget.NewProgressBarInfinite()
	mw.progressBar.Hide()

	mw.copyBtn = widget.NewButtonWithIcon("Copy Prompt(s)", theme.ContentCopyIcon(), mw.copyPrompts)
	mw.copyBtn.Importance = widget.HighImportance

	mw.saveBtn = widget.NewButtonWithIcon("Save To File", theme.DocumentSaveIcon(), mw.saveToFile)
	mw.saveBtn.Importance = widget.MediumImportance

	mw.clearBtn = widget.NewButtonWithIcon("Clear", theme.DeleteIcon(), mw.clearResults)
	mw.clearBtn.Importance = widget.LowImportance

	mw.aboutBtn = widget.NewButtonWithIcon("", theme.InfoIcon(), func() {
		showAbout(mw.window)
	})

	actions := container.NewHBox(
		layout.NewSpacer(),
		mw.copyBtn,
		mw.saveBtn,
		mw.clearBtn,
		layout.NewSpacer(),
	)

	versionLabel := widget.NewLabel("v" + AppVersion)
	versionLabel.TextStyle = fyne.TextStyle{Italic: true}

	mw.statusDot = canvas.NewCircle(theme.SuccessColor())
	mw.statusDot.Resize(fyne.NewSize(8, 8))
	mw.statusDot.Hide() // We'll show it when we have a state

	mw.statusLabel = widget.NewLabel("Ready")
	mw.statusLabel.TextStyle = fyne.TextStyle{Monospace: true}

	statusIndicator := container.NewHBox(
		container.NewCenter(mw.statusDot),
		mw.statusLabel,
	)

	footer := container.NewVBox(
		mw.progressBar,
		container.NewPadded(actions),
		container.NewHBox(
			mw.aboutBtn,
			versionLabel,
			layout.NewSpacer(),
			statusIndicator,
		),
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
		container.NewPadded(mainSplit),
	))

	mw.updateButtonStates()
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
		fyne.NewMenuItem("Copy Prompt(s)", mw.copyPrompts),
		fyne.NewMenuItem("Clear Results", mw.clearResults),
	)

	helpMenu := fyne.NewMenu("Help",
		fyne.NewMenuItem("Shortcuts", func() {
			showShortcuts(mw.window)
		}),
		fyne.NewMenuItem("About", func() {
			showAbout(mw.window)
		}),
	)

	mw.window.SetMainMenu(fyne.NewMainMenu(fileMenu, editMenu, helpMenu))
}

func (mw *MainWindow) toggleMode() {
	if mw.getMode() == "ComfyUI" {
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

func (mw *MainWindow) setBusy(busy bool) {
	mw.state.mu.Lock()
	defer mw.state.mu.Unlock()
	mw.state.busy = busy
}

func (mw *MainWindow) getMode() string {
	mw.state.mu.Lock()
	defer mw.state.mu.Unlock()
	return mw.state.mode
}

func (mw *MainWindow) setMode(mode string) {
	mw.state.mu.Lock()
	defer mw.state.mu.Unlock()
	mw.state.mode = mode
}

func (mw *MainWindow) getAutoCopy() bool {
	mw.state.mu.Lock()
	defer mw.state.mu.Unlock()
	return mw.state.autoCopy
}

func (mw *MainWindow) setAutoCopy(autoCopy bool) {
	mw.state.mu.Lock()
	defer mw.state.mu.Unlock()
	mw.state.autoCopy = autoCopy
}

func (mw *MainWindow) setCurrentFile(file string) {
	mw.state.mu.Lock()
	defer mw.state.mu.Unlock()
	mw.state.currentFile = file
}

func (mw *MainWindow) getCurrentFile() string {
	mw.state.mu.Lock()
	defer mw.state.mu.Unlock()
	return mw.state.currentFile
}

func (mw *MainWindow) setCurrent(result *extractor.ExtractionResult, promptTexts []string) {
	mw.state.mu.Lock()
	defer mw.state.mu.Unlock()
	mw.state.currentResult = result
	mw.state.promptTexts = promptTexts
}

func (mw *MainWindow) getPromptTextsCount() int {
	mw.state.mu.Lock()
	defer mw.state.mu.Unlock()
	return len(mw.state.promptTexts)
}

func (mw *MainWindow) getPromptTexts() []string {
	mw.state.mu.Lock()
	defer mw.state.mu.Unlock()
	if mw.state.promptTexts == nil {
		return nil
	}
	res := make([]string, len(mw.state.promptTexts))
	copy(res, mw.state.promptTexts)
	return res
}

type stateSnapshot struct {
	currentFile   string
	currentResult *extractor.ExtractionResult
	promptTexts   []string
	mode          string
	busy          bool
	autoCopy      bool
}

func (mw *MainWindow) snapshotForSave() stateSnapshot {
	mw.state.mu.Lock()
	defer mw.state.mu.Unlock()
	var promptTexts []string
	if mw.state.promptTexts != nil {
		promptTexts = make([]string, len(mw.state.promptTexts))
		copy(promptTexts, mw.state.promptTexts)
	}
	return stateSnapshot{
		currentFile:   mw.state.currentFile,
		currentResult: mw.state.currentResult,
		promptTexts:   promptTexts,
		mode:          mw.state.mode,
		busy:          mw.state.busy,
		autoCopy:      mw.state.autoCopy,
	}
}

func (mw *MainWindow) clearState() {
	mw.state.mu.Lock()
	defer mw.state.mu.Unlock()
	mw.state.busy = false
	mw.state.currentFile = ""
	mw.state.currentResult = nil
	mw.state.promptTexts = nil
	if mw.state.cancel != nil {
		mw.state.cancel()
		mw.state.cancel = nil
	}
}

func (mw *MainWindow) loadFile(path string) {
	ext := strings.ToLower(filepath.Ext(path))
	if ext != ".png" && ext != ".json" {
		return
	}

	mw.state.mu.Lock()
	if mw.state.cancel != nil {
		mw.state.cancel()
	}
	mw.state.currentFile = path
	mw.state.mu.Unlock()

	mw.processFile(path)
}

func (mw *MainWindow) setUIBusy(busy bool) {
	mw.setBusy(busy)

	if busy {
		mw.progressBar.Show()
		mw.statusDot.FillColor = theme.WarningColor()
		mw.statusDot.Show()
		mw.statusLabel.SetText("Processing...")
	} else {
		mw.progressBar.Hide()
	}
	mw.statusDot.Refresh()
	mw.updateButtonStates()
}

func (mw *MainWindow) releasePreviewImage() {
	if mw.previewImg != nil {
		mw.previewImg.Image = nil
		mw.previewImg.Refresh()
	}
}

func (mw *MainWindow) processFile(file string) {
	// Cancel any existing processing
	mw.state.mu.Lock()
	if mw.state.cancel != nil {
		mw.state.cancel()
	}
	// Use a background context that we can cancel
	ctx, cancel := context.WithCancel(context.Background())
	mw.state.cancel = cancel
	mw.state.currentFile = file
	mw.state.mu.Unlock()

	mw.setUIBusy(true)

	// Release previous preview image reference
	mw.releasePreviewImage()

	// Immediately clear old state
	mw.promptEntry.SetText("")
	mw.summaryEntry.SetText("")

	currentMode := mw.getMode()
	done := make(chan struct{})

	go func() {
		defer close(done)
		defer func() {
			if r := recover(); r != nil {
				log.Printf("[PANIC] %v", r)
				fyne.Do(func() {
					mw.setUIBusy(false)
					dialog.ShowError(fmt.Errorf("internal panic: %v", r), mw.window)
				})
			}
		}()

		var thumbImg image.Image
		var thumbW, thumbH int
		if strings.ToLower(filepath.Ext(file)) == ".png" {
			// Check if we were cancelled before thumb loading
			select {
			case <-ctx.Done():
				return
			default:
			}

			thumbCtx, thumbCancel := context.WithTimeout(ctx, 5*time.Second)
			select {
			case <-thumbCtx.Done():
				log.Printf("[DEBUG] Thumbnail loading cancelled or timed out")
			case res, ok := <-getThumbnailAsync(thumbCtx, file, 400):
				if ok {
					thumbImg, thumbW, thumbH = res.img, res.w, res.h
				}
			}
			thumbCancel()
		}

		// Check if we were cancelled during thumb loading
		select {
		case <-ctx.Done():
			return
		default:
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

		// Final check for cancellation before UI updates
		select {
		case <-ctx.Done():
			return
		default:
		}

		// Post all UI updates back on the main thread
		fyne.Do(func() {
			var label string
			if thumbImg != nil {
				ratioStr := calculateAspectRatio(thumbW, thumbH)
				label = fmt.Sprintf("[%s] %d×%d | %s", ratioStr, thumbW, thumbH, filepath.Base(file))
			}
			mw.previewImg.Image = thumbImg
			mw.previewImg.Refresh()
			mw.previewLabel.SetText(label)
			mw.onExtractionFinished(result)
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

	mw.setCurrent(result, allTexts)

	// Update UI on main thread
	mw.promptEntry.SetText(strings.Join(promptLines, "\n"))
	mw.summaryEntry.SetText(strings.Join(summaryLines, "\n"))
	mw.statusLabel.SetText(fmt.Sprintf("Found %d prompts in %s", len(allTexts), result.FileInfo.Filename))

	mw.tabs.Show()
	mw.emptyState.Hide()
	mw.resultsStack.Refresh()

	if result.Error != "" {
		mw.statusDot.FillColor = theme.ErrorColor()
	} else {
		mw.statusDot.FillColor = theme.SuccessColor()
	}
	mw.statusDot.Show()
	mw.statusDot.Refresh()

	if mw.getAutoCopy() && len(allTexts) > 0 {
		mw.copyPrompts()
		mw.statusLabel.SetText(mw.statusLabel.Text + " [COPIED TO CLIPBOARD]")
	}

	mw.setUIBusy(false)
}

func (mw *MainWindow) copyPrompts() {
	prompts := mw.getPromptTexts()
	if len(prompts) == 0 {
		return
	}
	text := strings.Join(prompts, "\n\n")
	clipboard.WriteAll(text)
}

func (mw *MainWindow) clearResults() {
	mw.clearState()

	// Stop recreating the two ReadOnlyEntry widgets; call SetText("") on existing ones.
	mw.promptEntry.SetText("")
	mw.summaryEntry.SetText("")

	// Reset preview
	mw.releasePreviewImage()
	mw.previewLabel.SetText("")

	mw.tabs.Hide()
	mw.emptyState.Show()
	mw.resultsStack.Refresh()

	mw.statusLabel.SetText("Ready")
	mw.statusDot.Hide()
	mw.statusDot.Refresh()
	mw.updateButtonStates()
}

func (mw *MainWindow) updateButtonStates() {
	mw.state.mu.Lock()
	hasResults := len(mw.state.promptTexts) > 0 && !mw.state.busy
	mw.state.mu.Unlock()
	if hasResults {
		mw.copyBtn.Enable()
		mw.saveBtn.Enable()
	} else {
		mw.copyBtn.Disable()
		mw.saveBtn.Disable()
	}

	// Clear button is always enabled as an emergency recovery path
	mw.clearBtn.Enable()
}

func (mw *MainWindow) saveToFile() {
	snap := mw.snapshotForSave()
	if len(snap.promptTexts) == 0 {
		return
	}

	base := strings.TrimSuffix(filepath.Base(snap.currentFile),
		filepath.Ext(snap.currentFile))
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
		fmt.Fprintf(writer, "\nExtractor mode: %s\n", snap.mode)
		fmt.Fprintf(writer, "File processed: %s\n", snap.currentFile)
		fmt.Fprintf(writer, "Total prompts: %d\n", len(snap.promptTexts))
		fmt.Fprintf(writer, "Extraction date: %s\n", time.Now().Format("2006-01-02 15:04:05"))
		fmt.Fprintln(writer, "\n"+strings.Repeat("=", 60))

		result := snap.currentResult
		if result != nil {
			for j, p := range result.PositivePrompts {
				if len(result.PositivePrompts) > 1 {
					fmt.Fprintf(writer, "Prompt %d - %s:\n%s\n", j+1, p.Title, strings.Repeat("-", 40))
				}
				fmt.Fprintln(writer, p.Text)
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

	// Use LimitReader to prevent decompression bombs or massive file reads (200MB)
	lr := io.LimitReader(f, 200*1024*1024)

	// 3.3. Use image.DecodeConfig before full decode
	config, _, err := image.DecodeConfig(lr)
	if err != nil {
		return nil, 0, 0, err
	}

	// Reset file pointer for full decode
	_, err = f.Seek(0, 0)
	if err != nil {
		return nil, 0, 0, err
	}
	// Re-wrap LimitReader after seek
	lr = io.LimitReader(f, 200*1024*1024)

	// Dimension guardrail: pixel budget instead of axis budget
	if config.Width*config.Height > 64000000 {
		return nil, config.Width, config.Height, fmt.Errorf("image dimensions too large (%dx%d)", config.Width, config.Height)
	}

	img, _, err := image.Decode(lr)
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
