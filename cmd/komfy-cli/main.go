package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/mamorett/goKomfy/internal/extractor"
)

// cliConfig holds all parsed command-line options for komfy-cli.
type cliConfig struct {
	files          []string
	mode           string // "ComfyUI" or "Parameters"
	modeExplicit   bool
	promptExplicit bool
	strict         bool
	raw            bool
	showResolution bool
	showRatio      bool
	showSummary    bool
	showAll        bool
	saveDefault    bool
	outputFile     string
	copyClipboard  bool
	jsonOutput     bool
	quiet          bool
	showHelp       bool
	showVersion    bool
}

// jsonFileOutput represents the JSON structure emitted with --json.
type jsonFileOutput struct {
	FileInfo         extractor.FileInfo     `json:"file_info"`
	ExtractorMode    string                 `json:"extractor_mode"`
	ExtractionMethod string                 `json:"extraction_method"`
	PositivePrompts  []extractor.PromptInfo `json:"positive_prompts"`
	RawMetadata      map[string]string      `json:"raw_metadata,omitempty"`
	Error            string                 `json:"error,omitempty"`
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		printDetailedHelp(stdout)
		return 0
	}

	cfg, err := parseArgs(args)
	if err != nil {
		fmt.Fprintf(stderr, "Error: %v\n\nRun 'komfy-cli --help' for detailed usage information.\n", err)
		return 2
	}

	if cfg.showHelp {
		printDetailedHelp(stdout)
		return 0
	}

	if cfg.showVersion {
		fmt.Fprintf(stdout, "komfy-cli (goKomfy) v%s\n", extractor.AppVersion)
		return 0
	}

	if len(cfg.files) == 0 {
		fmt.Fprintln(stderr, "Error: no input file specified.")
		fmt.Fprintln(stderr, "Run 'komfy-cli --help' for detailed usage information.")
		return 2
	}

	e := &extractor.PromptExtractor{}
	var jsonOutputs []jsonFileOutput
	var allClipboardPrompts []string
	exitCode := 0

	for idx, filePath := range cfg.files {
		if idx > 0 && !cfg.jsonOutput {
			fmt.Fprintln(stdout, strings.Repeat("=", 60))
		}

		res, rawMeta, fileErr := processFile(e, filePath, cfg)
		if fileErr != nil {
			exitCode = 1
			if cfg.jsonOutput {
				jsonOutputs = append(jsonOutputs, jsonFileOutput{
					FileInfo:      extractor.FileInfo{Filename: filepath.Base(filePath)},
					ExtractorMode: cfg.mode,
					Error:         fileErr.Error(),
				})
				continue
			}
			fmt.Fprintf(stderr, "Error processing %s: %v\n", filePath, fileErr)
			continue
		}

		if res != nil && res.Error != "" {
			exitCode = 1
			if cfg.jsonOutput {
				jsonOutputs = append(jsonOutputs, jsonFileOutput{
					FileInfo:         res.FileInfo,
					ExtractorMode:    cfg.mode,
					ExtractionMethod: res.ExtractionMethod,
					Error:            res.Error,
				})
				continue
			}
			fmt.Fprintf(stderr, "Error in %s: %s\n", res.FileInfo.Filename, res.Error)
			continue
		}

		// Collect prompts for clipboard if requested
		if res != nil {
			for _, p := range res.PositivePrompts {
				allClipboardPrompts = append(allClipboardPrompts, strings.TrimSpace(p.Text))
			}
		}

		// Handle file saving (--save or -o / --output)
		if cfg.saveDefault || cfg.outputFile != "" {
			savePath := cfg.outputFile
			if savePath == "" || len(cfg.files) > 1 {
				base := strings.TrimSuffix(filepath.Base(filePath), filepath.Ext(filePath))
				savePath = base + "_prompts.txt"
				if cfg.outputFile != "" && len(cfg.files) == 1 {
					savePath = cfg.outputFile
				}
			}
			if err := saveReportToFile(savePath, filePath, cfg.mode, res); err != nil {
				fmt.Fprintf(stderr, "Error saving report to %s: %v\n", savePath, err)
				exitCode = 1
			} else if !cfg.quiet && !cfg.jsonOutput {
				fmt.Fprintf(stderr, "Saved extraction report to %s\n", savePath)
			}
		}

		if cfg.jsonOutput {
			out := jsonFileOutput{
				FileInfo:         res.FileInfo,
				ExtractorMode:    cfg.mode,
				ExtractionMethod: res.ExtractionMethod,
				PositivePrompts:  res.PositivePrompts,
				RawMetadata:      rawMeta,
			}
			if out.PositivePrompts == nil {
				out.PositivePrompts = []extractor.PromptInfo{}
			}
			jsonOutputs = append(jsonOutputs, out)
			continue
		}

		// Render standard/CLI output
		if err := renderTextOutput(stdout, stderr, filePath, cfg, res, rawMeta, len(cfg.files) > 1); err != nil {
			exitCode = 1
		}
	}

	if cfg.jsonOutput {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		if len(jsonOutputs) == 1 {
			_ = enc.Encode(jsonOutputs[0])
		} else {
			_ = enc.Encode(jsonOutputs)
		}
	}

	if cfg.copyClipboard && len(allClipboardPrompts) > 0 {
		clipText := strings.Join(allClipboardPrompts, "\n\n")
		if err := copyToClipboard(clipText); err != nil {
			fmt.Fprintf(stderr, "Warning: failed to copy to clipboard: %v\n", err)
		} else if !cfg.quiet && !cfg.jsonOutput {
			fmt.Fprintln(stderr, "[COPIED TO CLIPBOARD]")
		}
	}

	return exitCode
}

func parseArgs(args []string) (*cliConfig, error) {
	cfg := &cliConfig{
		mode: "ComfyUI",
	}

	for i := 0; i < len(args); i++ {
		arg := args[i]

		if arg == "--" {
			cfg.files = append(cfg.files, args[i+1:]...)
			break
		}

		if arg == "help" && len(cfg.files) == 0 && i == 0 {
			cfg.showHelp = true
			return cfg, nil
		}

		if strings.HasPrefix(arg, "--") {
			name := arg[2:]
			var val string
			hasEq := false
			if eqIdx := strings.IndexByte(name, '='); eqIdx != -1 {
				val = name[eqIdx+1:]
				name = name[:eqIdx]
				hasEq = true
			}

			switch name {
			case "help", "h":
				cfg.showHelp = true
			case "version", "v":
				cfg.showVersion = true
			case "comfyui", "comfy":
				cfg.mode = "ComfyUI"
				cfg.modeExplicit = true
				cfg.promptExplicit = true
			case "parameters", "parameter", "params", "param":
				cfg.mode = "Parameters"
				cfg.modeExplicit = true
				cfg.promptExplicit = true
			case "mode":
				if !hasEq {
					if i+1 >= len(args) {
						return nil, errors.New("flag '--mode' requires an argument ('comfyui' or 'parameters')")
					}
					i++
					val = args[i]
				}
				normMode, err := normalizeMode(val)
				if err != nil {
					return nil, err
				}
				cfg.mode = normMode
				cfg.modeExplicit = true
				cfg.promptExplicit = true
			case "strict":
				cfg.strict = true
			case "raw":
				cfg.raw = true
			case "resolution", "res":
				cfg.showResolution = true
			case "ratio", "aspect-ratio", "aspect":
				cfg.showRatio = true
			case "summary":
				cfg.showSummary = true
			case "all":
				cfg.showAll = true
			case "save":
				cfg.saveDefault = true
			case "output", "out":
				if !hasEq {
					if i+1 >= len(args) {
						return nil, errors.New("flag '--output' requires a file path argument")
					}
					i++
					val = args[i]
				}
				if strings.TrimSpace(val) == "" {
					return nil, errors.New("flag '--output' cannot be empty")
				}
				cfg.outputFile = val
			case "copy", "clipboard":
				cfg.copyClipboard = true
			case "json":
				cfg.jsonOutput = true
			case "quiet", "no-header":
				cfg.quiet = true
			default:
				return nil, fmt.Errorf("unknown flag '--%s'", name)
			}
			continue
		}

		if strings.HasPrefix(arg, "-") && len(arg) > 1 {
			shorts := arg[1:]
			for j := 0; j < len(shorts); j++ {
				ch := shorts[j]
				switch ch {
				case 'h':
					cfg.showHelp = true
				case 'v':
					cfg.showVersion = true
				case 'c':
					cfg.mode = "ComfyUI"
					cfg.modeExplicit = true
					cfg.promptExplicit = true
				case 'p':
					cfg.mode = "Parameters"
					cfg.modeExplicit = true
					cfg.promptExplicit = true
				case 'm':
					var val string
					if j+1 < len(shorts) {
						val = shorts[j+1:]
						if strings.HasPrefix(val, "=") {
							val = val[1:]
						}
						j = len(shorts)
					} else {
						if i+1 >= len(args) {
							return nil, errors.New("flag '-m' requires an argument ('comfyui' or 'parameters')")
						}
						i++
						val = args[i]
					}
					normMode, err := normalizeMode(val)
					if err != nil {
						return nil, err
					}
					cfg.mode = normMode
					cfg.modeExplicit = true
					cfg.promptExplicit = true
				case 'r':
					cfg.showResolution = true
				case 'a':
					cfg.showRatio = true
				case 's':
					cfg.showSummary = true
				case 'A':
					cfg.showAll = true
				case 'o':
					var val string
					if j+1 < len(shorts) {
						val = shorts[j+1:]
						if strings.HasPrefix(val, "=") {
							val = val[1:]
						}
						j = len(shorts)
					} else {
						if i+1 >= len(args) {
							return nil, errors.New("flag '-o' requires a file path argument")
						}
						i++
						val = args[i]
					}
					if strings.TrimSpace(val) == "" {
						return nil, errors.New("flag '-o' cannot be empty")
					}
					cfg.outputFile = val
				case 'y':
					cfg.copyClipboard = true
				case 'j':
					cfg.jsonOutput = true
				case 'q':
					cfg.quiet = true
				default:
					return nil, fmt.Errorf("unknown shorthand flag '-%c'", ch)
				}
			}
			continue
		}

		cfg.files = append(cfg.files, arg)
	}

	return cfg, nil
}

func normalizeMode(val string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(val)) {
	case "comfyui", "comfy", "workflow", "prompt":
		return "ComfyUI", nil
	case "parameters", "parameter", "params", "param", "a1111":
		return "Parameters", nil
	default:
		return "", fmt.Errorf("invalid extraction mode %q (valid modes: 'comfyui', 'parameters')", val)
	}
}

func processFile(e *extractor.PromptExtractor, filePath string, cfg *cliConfig) (*extractor.ExtractionResult, map[string]string, error) {
	info, err := os.Stat(filePath)
	if err != nil {
		return nil, nil, err
	}
	if info.IsDir() {
		return nil, nil, fmt.Errorf("%s is a directory (expected a .png, .json, or .txt file)", filePath)
	}

	ext := strings.ToLower(filepath.Ext(filePath))
	if ext == ".png" && info.Size() > extractor.MaxPNGFileSize {
		return &extractor.ExtractionResult{
			FileInfo: extractor.FileInfo{Filename: filepath.Base(filePath), Mode: "PNG"},
			Error:    "File too large (> 200MB)",
		}, nil, nil
	}

	opts := &extractor.ExtractionOptions{
		Strict: cfg.strict,
	}

	var result *extractor.ExtractionResult
	switch ext {
	case ".json":
		result, err = e.ExtractJSON(filePath)
	case ".png":
		if cfg.mode == "Parameters" {
			result, err = e.ExtractParameters(filePath, opts)
		} else {
			result, err = e.ExtractComfyUI(filePath, opts)
		}
	case ".txt":
		result, err = e.ExtractText(filePath)
	default:
		return nil, nil, fmt.Errorf("unsupported file extension %q (supported: .png, .json, .txt)", ext)
	}

	if err != nil {
		return nil, nil, err
	}

	var rawMeta map[string]string
	if cfg.raw {
		rawMeta, err = e.ReadRawMetadata(filePath)
		if err != nil {
			return result, nil, err
		}
	}

	return result, rawMeta, nil
}

func renderTextOutput(stdout, stderr io.Writer, filePath string, cfg *cliConfig, result *extractor.ExtractionResult, rawMeta map[string]string, multiFile bool) error {
	if multiFile && !cfg.quiet {
		fmt.Fprintf(stdout, "File: %s\n", result.FileInfo.Filename)
	}

	wantRes := cfg.showResolution || cfg.showAll
	wantRatio := cfg.showRatio || cfg.showAll
	wantSummary := cfg.showSummary || cfg.showAll
	// Print prompts if explicitly requested (-c, -p, -m, -A) or if no specific metadata-only selector (-r, -a, -s, --raw) was given.
	wantPrompt := cfg.promptExplicit || cfg.showAll || (!cfg.showResolution && !cfg.showRatio && !cfg.showSummary && !cfg.raw)

	printedBlock := false

	// 1. Summary block (if --summary or --all)
	if wantSummary {
		fmt.Fprintln(stdout, extractor.FormatSummary(result, cfg.mode))
		printedBlock = true
	} else if wantRes || wantRatio {
		// 2. Standalone or combined Resolution / Aspect Ratio output
		if !result.FileInfo.HasDimensions() {
			fmt.Fprintf(stderr, "Error: %s does not contain image dimensions (mode: %s)\n", result.FileInfo.Filename, result.FileInfo.Mode)
			if !wantPrompt && !cfg.raw {
				return errors.New("no image dimensions available")
			}
		} else {
			if cfg.quiet {
				if wantRes && wantRatio {
					fmt.Fprintf(stdout, "%s\n%s\n", result.FileInfo.Resolution, result.FileInfo.AspectRatio)
				} else if wantRes {
					fmt.Fprintln(stdout, result.FileInfo.Resolution)
				} else if wantRatio {
					fmt.Fprintln(stdout, result.FileInfo.AspectRatio)
				}
			} else {
				if wantRes {
					fmt.Fprintf(stdout, "Resolution:   %s\n", result.FileInfo.Resolution)
				}
				if wantRatio {
					fmt.Fprintf(stdout, "Aspect Ratio: %s\n", result.FileInfo.AspectRatio)
				}
			}
			printedBlock = true
		}
	}

	// 3. Raw metadata output (if --raw)
	if cfg.raw {
		if printedBlock {
			fmt.Fprintln(stdout)
		}
		if err := renderRawMetadata(stdout, stderr, result.FileInfo.Filename, cfg, rawMeta); err != nil {
			return err
		}
		printedBlock = true
		if !cfg.promptExplicit && !cfg.showAll {
			return nil
		}
	}

	// 4. Positive Prompt(s) output
	if wantPrompt {
		if len(result.PositivePrompts) == 0 {
			fmt.Fprintf(stderr, "No prompts found in %s (mode: %s)\n", result.FileInfo.Filename, cfg.mode)
			return errors.New("no prompts found")
		}
		if printedBlock {
			fmt.Fprintln(stdout)
		}
		fmt.Fprintln(stdout, extractor.FormatPrompts(result.PositivePrompts, cfg.quiet))
	}

	return nil
}

func renderRawMetadata(stdout, stderr io.Writer, filename string, cfg *cliConfig, rawMeta map[string]string) error {
	if len(rawMeta) == 0 {
		fmt.Fprintf(stderr, "No raw metadata found in %s\n", filename)
		return errors.New("no raw metadata found")
	}

	if cfg.mode == "Parameters" {
		// Look for parameters, prompt, description, comment
		for _, key := range []string{"parameters", "Parameters", "Positive prompt", "prompt", "Description", "Comment"} {
			if val, ok := rawMeta[key]; ok && strings.TrimSpace(val) != "" {
				if !cfg.quiet {
					fmt.Fprintf(stdout, "[Raw %s]\n%s\n", key, strings.Repeat("-", 40))
				}
				fmt.Fprintln(stdout, strings.TrimSpace(val))
				return nil
			}
		}
		if cfg.strict {
			fmt.Fprintf(stderr, "No raw parameters metadata found in %s\n", filename)
			return errors.New("no raw parameters metadata found")
		}
	}

	// ComfyUI mode or fallback: check prompt and workflow chunks
	found := false
	for _, key := range []string{"prompt", "workflow", "json", "parameters"} {
		if val, ok := rawMeta[key]; ok && strings.TrimSpace(val) != "" {
			if found {
				fmt.Fprintln(stdout)
			}
			if !cfg.quiet {
				fmt.Fprintf(stdout, "[Raw %s]\n%s\n", key, strings.Repeat("-", 40))
			}
			// Pretty-print if valid JSON
			var parsed any
			if json.Unmarshal([]byte(val), &parsed) == nil {
				if pretty, err := json.MarshalIndent(parsed, "", "  "); err == nil {
					fmt.Fprintln(stdout, string(pretty))
					found = true
					continue
				}
			}
			fmt.Fprintln(stdout, strings.TrimSpace(val))
			found = true
		}
	}

	if !found {
		for k, v := range rawMeta {
			if strings.TrimSpace(v) != "" {
				if !cfg.quiet {
					fmt.Fprintf(stdout, "[Raw %s]\n%s\n", k, strings.Repeat("-", 40))
				}
				fmt.Fprintln(stdout, strings.TrimSpace(v))
				found = true
			}
		}
	}

	if !found {
		fmt.Fprintf(stderr, "No raw metadata found in %s\n", filename)
		return errors.New("no raw metadata found")
	}
	return nil
}

func saveReportToFile(savePath, sourceFile, mode string, result *extractor.ExtractionResult) error {
	f, err := os.Create(savePath)
	if err != nil {
		return err
	}
	defer f.Close()
	return extractor.WriteExtractionReport(f, sourceFile, mode, result)
}

func copyToClipboard(text string) error {
	type clipCmd struct {
		name string
		args []string
	}

	var candidates []clipCmd
	switch runtime.GOOS {
	case "darwin":
		candidates = []clipCmd{{"pbcopy", nil}}
	case "windows":
		candidates = []clipCmd{{"clip", nil}}
	default:
		if os.Getenv("WAYLAND_DISPLAY") != "" {
			candidates = append(candidates, clipCmd{"wl-copy", nil})
		}
		candidates = append(candidates,
			clipCmd{"xclip", []string{"-selection", "clipboard"}},
			clipCmd{"xsel", []string{"--clipboard", "--input"}},
			clipCmd{"wl-copy", nil},
		)
	}

	for _, c := range candidates {
		path, err := exec.LookPath(c.name)
		if err != nil {
			continue
		}
		cmd := exec.Command(path, c.args...)
		cmd.Stdin = strings.NewReader(text)
		if err := cmd.Run(); err == nil {
			return nil
		}
	}

	return errors.New("no supported clipboard utility found (install wl-clipboard, xclip, or xsel)")
}

func printDetailedHelp(w io.Writer) {
	help := fmt.Sprintf(`================================================================================
  komfy-cli (goKomfy) v%s — ComfyUI & Stable Diffusion Prompt Extractor
================================================================================

DESCRIPTION
  komfy-cli is the full-featured 1:1 command-line companion to the goKomfy GUI.
  It extracts positive prompts, generation parameters, image resolution, and
  aspect ratio from AI-generated PNG images, ComfyUI JSON workflows, and text
  parameter files without decoding pixel data into memory.

USAGE
  komfy-cli [OPTIONS] <file> [file...]
  gokomfy-cli [OPTIONS] <file> [file...]

EXTRACTION MODE & PROMPT OPTIONS
  -c, --comfyui              Extract and print ComfyUI positive prompt(s).
                             Prioritizes 'workflow' and 'prompt' PNG chunks
                             (CLIPTextEncode nodes) and falls back to
                             'parameters' metadata if none are found. [Default]

  -p, --parameters           Extract and print Parameters positive prompt(s).
      --parameter            Prioritizes Automatic1111/Forge 'parameters' and
                             PNG text properties ('Positive prompt', 'Prompt',
                             'Description', 'Comment'), falling back to ComfyUI
                             workflow/prompt metadata if none are found.

  -m, --mode <mode>          Explicitly set extraction mode:
                               • comfyui     (ComfyUI workflow/prompt priority)
                               • parameters  (A1111/Forge parameters priority)

      --strict               Disable automatic cross-mode fallback. When used
                             with --comfyui, only ComfyUI nodes are inspected;
                             when used with --parameters, only parameter chunks
                             are inspected.

      --raw                  Print the raw, unparsed metadata chunk from the
                             file ('prompt'/'workflow' JSON in ComfyUI mode, or
                             the full 'parameters' block including negative
                             prompts, steps, sampler, and seed in Parameters
                             mode).

IMAGE GEOMETRY OPTIONS (RESOLUTION & ASPECT RATIO)
  -r, --resolution           Print image resolution in WIDTHxHEIGHT format
                             (e.g., 1024x1024, 1920x1080). When used without
                             -c or -p, prints only the resolution.

  -a, --ratio                Print calculated image aspect ratio (e.g., 1:1,
      --aspect-ratio         16:9, 9:16, 4:3, 3:4, 3:2, 2:3, 9:7, 7:9, or
                             exact GCD ratio). When used without -c or -p,
                             prints only the aspect ratio.

OUTPUT & ACTION OPTIONS (1:1 WITH KOMFY GUI)
  -s, --summary              Print the extraction summary (File, Mode, Method,
                             Resolution, Aspect Ratio, and Prompts count),
                             matching the Komfy GUI Summary tab.

  -A, --all                  Print all information: Extraction Summary,
                             Resolution, Aspect Ratio, and Extracted Prompts.

      --save                 Save formatted extraction report to
                             '<basename>_prompts.txt' (1:1 with the Komfy GUI
                             "Save To File" action).

  -o, --output <file>        Save formatted extraction report to the specified
                             file path.

  -y, --copy                 Copy extracted positive prompt(s) directly to the
      --clipboard            system clipboard (uses wl-copy, xclip, xsel,
                             pbcopy, or clip).

  -j, --json                 Output all extracted data (file info, resolution,
                             aspect ratio, extraction method, and prompts) as
                             formatted JSON.

  -q, --quiet                Quiet / script-friendly mode. Suppresses headers,
      --no-header            labels ('Resolution:', 'Aspect Ratio:', 'Prompt 1'),
                             and divider lines for clean shell piping.

GENERAL OPTIONS
  -v, --version              Print version information and exit.
  -h, --help                 Print this detailed help message and exit.

SUPPORTED FORMATS & EXTRACTION PIPELINE
  • .png   Reads PNG header (IHDR) for instant resolution & aspect ratio
           without pixel decoding (up to 200 MB limit), then parses tEXt,
           zTXt (zlib-compressed), and iTXt (international UTF-8) chunks.
  • .json  Parses exported ComfyUI workflow files ('nodes' array) as well as
           ComfyUI API prompt format ('class_type' / 'inputs' map).
  • .txt   Parses standalone Automatic1111 / Forge parameter text files.

EXAMPLES
  1. Extract positive prompt (default ComfyUI mode with automatic fallback):
     $ komfy-cli image.png

  2. Explicitly extract ComfyUI prompt or A1111/Forge parameter prompt:
     $ komfy-cli --comfyui image.png
     $ komfy-cli --parameters image.png
     $ komfy-cli -p image.png

  3. Print only the image resolution, aspect ratio, or both:
     $ komfy-cli --resolution image.png
     $ komfy-cli --ratio image.png
     $ komfy-cli -r -a image.png

  4. Print ratio and resolution together with the ComfyUI or Parameters prompt:
     $ komfy-cli -c -r -a image.png
     $ komfy-cli -p -r -a image.png

  5. Quiet output for shell scripts and variable capture:
     $ RES=$(komfy-cli -q -r image.png)
     $ RATIO=$(komfy-cli -q -a image.png)

  6. Print full summary + prompts, save report, and copy to clipboard:
     $ komfy-cli --all --save --copy image.png

  7. Inspect raw PNG metadata (full parameters block or ComfyUI JSON):
     $ komfy-cli --raw --parameters image.png
     $ komfy-cli --raw --comfyui image.png

  8. Export structured JSON for automated pipelines:
     $ komfy-cli --json image.png

EXIT CODES
  0  Success
  1  Extraction error, file error, or no matching prompts/metadata found
  2  Invalid command-line arguments or missing input file
`, extractor.AppVersion)

	fmt.Fprint(w, help)
}
