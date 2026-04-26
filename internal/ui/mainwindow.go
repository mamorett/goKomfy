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
	"runtime"
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
		mw.state.mode = s
		if mw.state.currentFile != "" && !mw.isBusy() {
			mw.processFile(mw.state.currentFile)
		}
	})
	mw.modeSelect.SetSelected(mw.state.mode)

	mw.autoCopyCheck = widget.NewCheck("Auto-copy", func(b bool) {
		mw.state.autoCopy = b
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

	mw.previewCont = container.NewBorder(nil, mw.previewLabel, nil, nil, mw.previewBoxCont)
	mw.previewCont.Hide()

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

	actions := container.NewHBox(
		layout.NewSpacer(),
		mw.copyBtn,
		mw.saveBtn,
		mw.clearBtn,
		layout.NewSpacer(),
	)

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
		container.NewHBox(layout.NewSpacer(), statusIndicator),
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
	mw.state.mu.Unlock()

	mw.setUIBusy(true)
	mw.state.currentFile = file

	// Release previous preview image reference
	mw.releasePreviewImage()

	// Immediately clear old state and hide preview
	mw.promptEntry.SetText("")
	mw.summaryEntry.SetText("")
	mw.previewCont.Hide()

	currentMode := mw.state.mode

	// Safety net goroutine: integrated with the same context
	go func() {
		timer := time.NewTimer(130 * time.Second)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			// Normal cancellation or completion
			return
		case <-timer.C:
			// If we hit 130s and are still busy, something is likely stuck
			if mw.isBusy() {
				log.Printf("[WARN] Processing safety net triggered after 130s")
				fyne.Do(func() {
					mw.setUIBusy(false)
					mw.statusLabel.SetText("Processing timed out - recovered")
				})
			}
		}
	}()

	go func() {
		// Ensure we cancel on exit if not already done, but we usually want to 
		// keep it until setUIBusy(false) is called or next file is dropped.
		// However, once this goroutine finishes, we should clear the cancel func
		// if it's still ours.
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
			select {
			case <-ctx.Done():
				log.Printf("[DEBUG] Thumbnail loading cancelled")
				return
			case res, ok := <-getThumbnailAsync(ctx, file, 400):
				if ok {
					thumbImg, thumbW, thumbH = res.img, res.w, res.h
				}
			}
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

	if mw.state.autoCopy && len(allTexts) > 0 {
		mw.copyPrompts()
		mw.statusLabel.SetText(mw.statusLabel.Text + " [COPIED TO CLIPBOARD]")
	}

	mw.setUIBusy(false)
	runtime.GC()
}

func (mw *MainWindow) copyPrompts() {
	if len(mw.state.promptTexts) == 0 {
		return
	}
	text := strings.Join(mw.state.promptTexts, "\n\n")
	clipboard.WriteAll(text)
}

func (mw *MainWindow) clearResults() {
	mw.state.mu.Lock()
	mw.state.currentFile = ""
	mw.state.currentResult = nil
	mw.state.promptTexts = nil
	if mw.state.cancel != nil {
		mw.state.cancel()
	}
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
	mw.releasePreviewImage()
	newImg := canvas.NewImageFromImage(nil)
	newImg.FillMode = canvas.ImageFillContain
	mw.previewBoxCont.Objects[0] = newImg
	mw.previewBoxCont.Refresh()
	mw.previewImg = newImg
	mw.previewCont.Hide()

	mw.tabs.Hide()
	mw.emptyState.Show()
	mw.resultsStack.Refresh()

	mw.statusLabel.SetText("Ready")
	mw.statusDot.Hide()
	mw.statusDot.Refresh()
	mw.updateButtonStates()
	runtime.GC()
}

func (mw *MainWindow) updateButtonStates() {
	hasResults := len(mw.state.promptTexts) > 0 && !mw.state.busy
	if hasResults {
		mw.copyBtn.Enable()
		mw.saveBtn.Enable()
	} else {
		mw.copyBtn.Disable()
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

	// Dimension guardrail
	if config.Width > 12000 || config.Height > 12000 {
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
	runtime.GC()

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

