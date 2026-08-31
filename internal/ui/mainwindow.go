package ui

import (
	"bufio"
	"context"
	"fmt"
	"image"
	"image/color"
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
	// maxThumbnailSourcePixels bounds full-resolution decoding for the optional
	// preview. Extraction itself never decodes pixels; this only caps preview cost.
	maxThumbnailSourcePixels = 8_000_000
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

type loadJob struct {
	path string
	mode string
	gen  uint64
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

	// Load-worker state. This bounds concurrency to a single active job plus one
	// pending (latest-wins) job, so rapid drops can never pile up decodes.
	loadMu        sync.Mutex
	nextGen       uint64
	latestJob     *loadJob
	jobWake       chan struct{}
	stopWorker    chan struct{}
	currentCancel context.CancelFunc
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

	mw.jobWake = make(chan struct{}, 1)
	mw.stopWorker = make(chan struct{})
	go mw.runLoadWorker()

	return mw
}

func (mw *MainWindow) setupUI() {
	// 1. Header (Mode + Browse)
	mw.modeSelect = widget.NewSelect([]string{"ComfyUI", "Parameters"}, func(s string) {
		mw.setMode(s)
		curFile := mw.getCurrentFile()
		if curFile != "" {
			mw.loadFile(curFile)
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
}

func (mw *MainWindow) loadFile(path string) {
	ext := strings.ToLower(filepath.Ext(path))
	if ext != ".png" && ext != ".json" {
		return
	}
	mw.queue(path)
}

// queue records the latest requested file and wakes the load worker. Latest-wins:
// a pending job is overwritten and the currently running job is cancelled (its
// running decode can't be aborted, but its remaining work is skipped).
func (mw *MainWindow) queue(path string) {
	mw.loadMu.Lock()
	mw.nextGen++
	mw.latestJob = &loadJob{path: path, mode: mw.getMode(), gen: mw.nextGen}
	if mw.currentCancel != nil {
		mw.currentCancel()
	}
	mw.loadMu.Unlock()

	select {
	case mw.jobWake <- struct{}{}:
	default:
	}
}

// cancelPendingLoad cancels the active job and drops the queued one. Used by
// Clear so a cancelled-but-in-flight job can't repopulate the UI afterwards.
func (mw *MainWindow) cancelPendingLoad() {
	mw.loadMu.Lock()
	mw.nextGen++ // invalidate any in-flight job so it won't repopulate the UI
	mw.latestJob = nil
	if mw.currentCancel != nil {
		mw.currentCancel()
	}
	mw.loadMu.Unlock()
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

// runLoadWorker is the single background worker that processes drops. It runs
// exactly one job at a time and keeps at most one pending job (latest-wins),
// which is what finally bounds the memory/CPU used by preview decodes.
func (mw *MainWindow) runLoadWorker() {
	for {
		mw.loadMu.Lock()
		job := mw.latestJob
		mw.latestJob = nil
		mw.loadMu.Unlock()

		if job == nil {
			select {
			case <-mw.jobWake:
			case <-mw.stopWorker:
				return
			}
			continue
		}

		mw.doProcessFile(job)
	}
}

func (mw *MainWindow) doProcessFile(job *loadJob) {
	ctx, cancel := context.WithCancel(context.Background())

	mw.loadMu.Lock()
	mw.currentCancel = cancel
	mw.loadMu.Unlock()

	defer cancel()

	defer func() {
		if r := recover(); r != nil {
			log.Printf("[PANIC] %v", r)
			fyne.Do(func() {
				mw.setUIBusy(false)
				dialog.ShowError(fmt.Errorf("internal panic: %v", r), mw.window)
			})
		}
	}()

	mw.setCurrentFile(job.path)

	// Widget mutation must happen on the UI thread. Queue the "start" state, then
	// do all heavy work here on the worker goroutine.
	fyne.Do(func() {
		mw.setUIBusy(true)
		mw.releasePreviewImage()
		mw.promptEntry.SetText("")
		mw.summaryEntry.SetText("")
	})

	ext := strings.ToLower(filepath.Ext(job.path))

	var thumbImg image.Image
	var thumbW, thumbH int
	thumbTooLarge := false
	if ext == ".png" {
		if ctx.Err() != nil {
			return
		}
		var terr error
		thumbImg, thumbW, thumbH, thumbTooLarge, terr = decodeThumbnail(ctx, job.path, 400)
		if terr != nil && !thumbTooLarge {
			log.Printf("[DEBUG] thumbnail failed for %s: %v", job.path, terr)
			thumbImg, thumbW, thumbH = nil, 0, 0
		}
	}

	if ctx.Err() != nil {
		return
	}

	// Run extraction (metadata only; never decodes pixels).
	e := &extractor.PromptExtractor{}
	var result *extractor.ExtractionResult
	var err error

	// Reject oversized PNG files before processing.
	if ext == ".png" {
		info, errStat := os.Stat(job.path)
		if errStat == nil && info.Size() > 200*1024*1024 { // 200MB limit
			result = &extractor.ExtractionResult{
				FileInfo: extractor.FileInfo{Filename: filepath.Base(job.path)},
				Error:    "File too large (> 200MB)",
			}
		}
	}

	if result == nil {
		var opts *extractor.ExtractionOptions
		if thumbW > 0 && thumbH > 0 {
			opts = &extractor.ExtractionOptions{Width: thumbW, Height: thumbH}
		}

		switch ext {
		case ".json":
			result, err = e.ExtractJSON(job.path)
		case ".png":
			if job.mode == "ComfyUI" {
				result, err = e.ExtractComfyUI(job.path, opts)
			} else {
				result, err = e.ExtractParameters(job.path, opts)
			}
		}

		if err != nil {
			result = &extractor.ExtractionResult{
				FileInfo: extractor.FileInfo{Filename: filepath.Base(job.path)},
				Error:    err.Error(),
			}
		}
	}

	// Post the final UI update on the main thread. Skip it if a newer job or a
	// Clear superseded this one while it was processing.
	fyne.Do(func() {
		mw.loadMu.Lock()
		stale := job.gen != mw.nextGen
		mw.loadMu.Unlock()
		if stale {
			return
		}

		var label string
		switch {
		case thumbImg != nil:
			ratioStr := calculateAspectRatio(thumbW, thumbH)
			label = fmt.Sprintf("[%s] %d×%d | %s", ratioStr, thumbW, thumbH, filepath.Base(job.path))
		case thumbTooLarge:
			ratioStr := calculateAspectRatio(thumbW, thumbH)
			label = fmt.Sprintf("[%s] %d×%d | %s — preview unavailable", ratioStr, thumbW, thumbH, filepath.Base(job.path))
		}
		mw.previewImg.Image = thumbImg
		mw.previewImg.Refresh()
		mw.previewLabel.SetText(label)
		mw.onExtractionFinished(result)
	})
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
	// Drop any queued job and cancel the running one so it can't repopulate on finish.
	mw.cancelPendingLoad()
	mw.clearState()
	mw.setUIBusy(false)

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

func decodeThumbnail(ctx context.Context, filePath string, maxSize int) (image.Image, int, int, bool, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return nil, 0, 0, false, err
	}
	defer f.Close()

	// DecodeConfig only parses the header — no pixel decode and no big allocation.
	lr := io.LimitReader(f, 200*1024*1024)
	config, _, err := image.DecodeConfig(lr)
	if err != nil {
		return nil, 0, 0, false, err
	}
	w, h := config.Width, config.Height

	// Never fully decode an unreasonably large image just for a 400px preview.
	// Extraction doesn't need the pixels; this only caps the preview cost.
	if w*h > maxThumbnailSourcePixels {
		return nil, w, h, true, nil
	}

	if ctx.Err() != nil {
		return nil, w, h, false, ctx.Err()
	}

	if _, err := f.Seek(0, 0); err != nil {
		return nil, w, h, false, err
	}
	lr = io.LimitReader(f, 200*1024*1024)

	img, _, err := image.Decode(lr)
	if err != nil {
		return nil, w, h, false, err
	}

	origW := img.Bounds().Dx()
	origH := img.Bounds().Dy()

	// Scale down maintaining aspect ratio.
	scale := float64(maxSize) / math.Max(float64(origW), float64(origH))
	if scale >= 1.0 {
		return img, origW, origH, false, nil
	}
	newW := int(float64(origW) * scale)
	newH := int(float64(origH) * scale)
	dst := image.NewRGBA(image.Rect(0, 0, newW, newH))
	draw.BiLinear.Scale(dst, dst.Bounds(), img, img.Bounds(), draw.Over, nil)

	return dst, origW, origH, false, nil
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
