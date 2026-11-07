package extractor

import (
	"bufio"
	"encoding/json"
	"fmt"
	"image"
	_ "image/png"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mamorett/goKomfy/internal/png"
)

const (
	// AppVersion is the shared version across komfy and komfy-cli.
	AppVersion = "4.1.0"

	// MaxPNGFileSize caps the maximum PNG file size (200 MB).
	MaxPNGFileSize = 200 * 1024 * 1024
)

// FileInfo describes the source file.
type FileInfo struct {
	Filename    string `json:"filename"`
	Width       int    `json:"width,omitempty"`
	Height      int    `json:"height,omitempty"`
	Resolution  string `json:"resolution,omitempty"`
	AspectRatio string `json:"aspect_ratio,omitempty"`
	Mode        string `json:"mode"` // "PNG", "JSON", "TEXT"
}

// HasDimensions returns true if the file is an image with valid dimensions.
func (fi FileInfo) HasDimensions() bool {
	return fi.Mode == "PNG" && fi.Width > 0 && fi.Height > 0
}

// PromptInfo holds one extracted positive prompt.
type PromptInfo struct {
	Text     string `json:"text"`
	NodeID   string `json:"node_id"`
	NodeType string `json:"node_type"`
	Title    string `json:"title"`
	Source   string `json:"source"`
}

// ExtractionResult is the result for one file.
type ExtractionResult struct {
	FileInfo         FileInfo     `json:"file_info"`
	PositivePrompts  []PromptInfo `json:"positive_prompts"`
	ExtractionMethod string       `json:"extraction_method"`
	Error            string       `json:"error,omitempty"`
}

// ExtractionOptions allows passing pre-calculated dimensions and strict mode settings.
type ExtractionOptions struct {
	Width  int
	Height int
	Strict bool
}

type PromptExtractor struct{}

func (e *PromptExtractor) ExtractComfyUI(filePath string, opts ...*ExtractionOptions) (*ExtractionResult, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var w, h int
	var strict bool
	if len(opts) > 0 && opts[0] != nil {
		strict = opts[0].Strict
		if opts[0].Width > 0 {
			w = opts[0].Width
			h = opts[0].Height
		}
	}
	if w == 0 || h == 0 {
		config, _, err := image.DecodeConfig(f)
		if err != nil {
			return nil, err
		}
		w = config.Width
		h = config.Height
		// Seek back to start for metadata reading
		if _, err := f.Seek(0, 0); err != nil {
			return nil, err
		}
	}

	meta, err := png.ReadTextChunksFromReader(f)
	if err != nil {
		return nil, err
	}

	result := &ExtractionResult{
		FileInfo: FileInfo{
			Filename:    filepath.Base(filePath),
			Width:       w,
			Height:      h,
			Resolution:  fmt.Sprintf("%dx%d", w, h),
			AspectRatio: CalculateAspectRatio(w, h),
			Mode:        "PNG",
		},
		ExtractionMethod: "comfyui",
	}

	processedNodes := make(map[any]bool)

	// Try workflow first
	if workflowJSON, ok := meta["workflow"]; ok {
		var workflowData map[string]any
		if err := json.Unmarshal([]byte(workflowJSON), &workflowData); err == nil {
			prompts := e.extractPositiveFromWorkflow(workflowData, processedNodes)
			result.PositivePrompts = append(result.PositivePrompts, prompts...)
		}
	}

	// Then prompt data if none found
	if len(result.PositivePrompts) == 0 {
		if promptJSON, ok := meta["prompt"]; ok {
			var promptData map[string]any
			if err := json.Unmarshal([]byte(promptJSON), &promptData); err == nil {
				prompts := e.extractPositiveFromPromptData(promptData, processedNodes)
				result.PositivePrompts = append(result.PositivePrompts, prompts...)
			}
		}
	}

	// Fallback to parameters/PNG properties if no ComfyUI metadata found (unless strict mode)
	if len(result.PositivePrompts) == 0 && !strict {
		if promptText, ok := e.extractPositiveFromParametersStrict(meta); ok {
			result.PositivePrompts = append(result.PositivePrompts, PromptInfo{
				Text:     promptText,
				NodeID:   "parameters",
				NodeType: "parameters",
				Title:    "Parameters",
				Source:   "parameters",
			})
			result.ExtractionMethod = "parameters"
		} else if promptText, ok := e.extractPositiveFromPNGProperties(meta); ok {
			result.PositivePrompts = append(result.PositivePrompts, PromptInfo{
				Text:     promptText,
				NodeID:   "png_properties",
				NodeType: "png_properties",
				Title:    "PNG Properties",
				Source:   "png_properties",
			})
			result.ExtractionMethod = "png_properties"
		}
	}

	return result, nil
}

func (e *PromptExtractor) ExtractParameters(filePath string, opts ...*ExtractionOptions) (*ExtractionResult, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var w, h int
	var strict bool
	if len(opts) > 0 && opts[0] != nil {
		strict = opts[0].Strict
		if opts[0].Width > 0 {
			w = opts[0].Width
			h = opts[0].Height
		}
	}
	if w == 0 || h == 0 {
		config, _, err := image.DecodeConfig(f)
		if err != nil {
			return nil, err
		}
		w = config.Width
		h = config.Height
		// Seek back to start for metadata reading
		if _, err := f.Seek(0, 0); err != nil {
			return nil, err
		}
	}

	meta, err := png.ReadTextChunksFromReader(f)
	if err != nil {
		return nil, err
	}

	result := &ExtractionResult{
		FileInfo: FileInfo{
			Filename:    filepath.Base(filePath),
			Width:       w,
			Height:      h,
			Resolution:  fmt.Sprintf("%dx%d", w, h),
			AspectRatio: CalculateAspectRatio(w, h),
			Mode:        "PNG",
		},
		ExtractionMethod: "parameters",
	}

	// First, try the parameters extraction
	if promptText, ok := e.extractPositiveFromParametersStrict(meta); ok {
		result.PositivePrompts = append(result.PositivePrompts, PromptInfo{
			Text:     promptText,
			NodeID:   "parameters",
			NodeType: "parameters",
			Title:    "Parameters",
			Source:   "parameters",
		})
	} else if promptText, ok := e.extractPositiveFromPNGProperties(meta); ok {
		result.PositivePrompts = append(result.PositivePrompts, PromptInfo{
			Text:     promptText,
			NodeID:   "png_properties",
			NodeType: "png_properties",
			Title:    "PNG Properties",
			Source:   "png_properties",
		})
		result.ExtractionMethod = "png_properties"
	} else if !strict {
		// Fallback to ComfyUI workflow/prompt if Parameters mode selected on a ComfyUI image
		processedNodes := make(map[any]bool)
		if workflowJSON, ok := meta["workflow"]; ok {
			var workflowData map[string]any
			if err := json.Unmarshal([]byte(workflowJSON), &workflowData); err == nil {
				prompts := e.extractPositiveFromWorkflow(workflowData, processedNodes)
				result.PositivePrompts = append(result.PositivePrompts, prompts...)
			}
		}
		if len(result.PositivePrompts) == 0 {
			if promptJSON, ok := meta["prompt"]; ok {
				var promptData map[string]any
				if err := json.Unmarshal([]byte(promptJSON), &promptData); err == nil {
					prompts := e.extractPositiveFromPromptData(promptData, processedNodes)
					result.PositivePrompts = append(result.PositivePrompts, prompts...)
				}
			}
		}
		if len(result.PositivePrompts) > 0 {
			result.ExtractionMethod = "comfyui"
		}
	}

	return result, nil
}

func (e *PromptExtractor) ExtractJSON(filePath string) (*ExtractionResult, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	var jsonData map[string]any
	if err := json.Unmarshal(data, &jsonData); err != nil {
		return nil, err
	}

	result := &ExtractionResult{
		FileInfo: FileInfo{
			Filename: filepath.Base(filePath),
			Mode:     "JSON",
		},
		ExtractionMethod: "json",
	}

	processedNodes := make(map[any]bool)

	// Try as workflow (has 'nodes' list)
	if _, ok := jsonData["nodes"]; ok {
		prompts := e.extractPositiveFromWorkflow(jsonData, processedNodes)
		result.PositivePrompts = append(result.PositivePrompts, prompts...)
	}

	// Try as API format
	if len(result.PositivePrompts) == 0 {
		isAPIFormat := false
		for _, v := range jsonData {
			if node, ok := v.(map[string]any); ok {
				if _, ok := node["class_type"]; ok {
					isAPIFormat = true
					break
				}
				if _, ok := node["inputs"]; ok {
					isAPIFormat = true
					break
				}
			}
		}

		if isAPIFormat {
			prompts := e.extractPositiveFromPromptData(jsonData, processedNodes)
			result.PositivePrompts = append(result.PositivePrompts, prompts...)
		}
	}

	return result, nil
}

func (e *PromptExtractor) ExtractText(filePath string) (*ExtractionResult, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return &ExtractionResult{
			FileInfo: FileInfo{Filename: filepath.Base(filePath)},
			Error:    err.Error(),
		}, nil
	}

	textContent := string(data)
	result := &ExtractionResult{
		FileInfo: FileInfo{
			Filename: filepath.Base(filePath),
			Width:    len(textContent),
			Mode:     "TEXT",
		},
		ExtractionMethod: "text_parameters",
	}

	fakeMeta := map[string]string{"parameters": textContent}
	if promptText, ok := e.extractPositiveFromParametersStrict(fakeMeta); ok {
		result.PositivePrompts = append(result.PositivePrompts, PromptInfo{
			Text:     promptText,
			NodeID:   "parameters",
			NodeType: "parameters",
			Title:    "Parameters",
			Source:   "text_parameters",
		})
	}

	return result, nil
}

func (e *PromptExtractor) extractPositiveFromWorkflow(workflowData map[string]any, processed map[any]bool) []PromptInfo {
	var positivePrompts []PromptInfo
	nodesRaw, ok := workflowData["nodes"]
	if !ok {
		return nil
	}
	nodes, ok := nodesRaw.([]any)
	if !ok {
		return nil
	}

	for _, n := range nodes {
		node, ok := n.(map[string]any)
		if !ok {
			continue
		}

		nodeID := node["id"]
		nodeType, _ := node["type"].(string)
		title, _ := node["title"].(string)
		titleLower := strings.ToLower(title)

		if processed[nodeID] {
			continue
		}

		isCLIP := nodeType == "CLIPTextEncode" || strings.Contains(strings.ToLower(nodeType), "cliptext")
		if !isCLIP {
			if props, ok := node["properties"].(map[string]any); ok {
				if name, ok := props["Node name for S&R"].(string); ok && name == "CLIPTextEncode" {
					isCLIP = true
				}
			}
		}

		if isCLIP {
			widgetsValuesRaw, ok := node["widgets_values"]
			if !ok {
				continue
			}
			widgetsValues, ok := widgetsValuesRaw.([]any)
			if !ok || len(widgetsValues) == 0 {
				continue
			}

			promptTextRaw := widgetsValues[0]
			promptText := ""
			switch v := promptTextRaw.(type) {
			case string:
				promptText = v
			case []any:
				var sb strings.Builder
				for i, item := range v {
					if i > 0 {
						sb.WriteString("\n")
					}
					sb.WriteString(fmt.Sprintf("%v", item))
				}
				promptText = sb.String()
			case float64, int:
				promptText = fmt.Sprintf("%v", v)
			default:
				continue
			}

			promptTextLower := strings.ToLower(promptText)
			promptTextTrimmed := strings.TrimSpace(promptTextLower)

			isPositive := strings.Contains(titleLower, "positive") ||
				strings.Contains(titleLower, "pos") ||
				((title == "" || titleLower == "untitled") && promptTextTrimmed != "" && !strings.Contains(promptTextLower[:min(50, len(promptTextLower))], "negative"))

			isNegative := strings.Contains(titleLower, "negative") ||
				strings.Contains(titleLower, "neg") ||
				promptTextTrimmed == "" ||
				strings.HasPrefix(promptTextTrimmed, "negative")

			if isPositive && !isNegative {
				nodeTitle := title
				if nodeTitle == "" {
					nodeTitle = "Untitled"
				}
				positivePrompts = append(positivePrompts, PromptInfo{
					Text:     promptText,
					NodeID:   fmt.Sprintf("%v", nodeID),
					NodeType: nodeType,
					Title:    nodeTitle,
					Source:   "workflow",
				})
				processed[nodeID] = true
			}
		}
	}

	return positivePrompts
}

func (e *PromptExtractor) extractPositiveFromPromptData(promptData map[string]any, processed map[any]bool) []PromptInfo {
	var positivePrompts []PromptInfo

	for key, v := range promptData {
		value, ok := v.(map[string]any)
		if !ok {
			continue
		}

		classType, _ := value["class_type"].(string)

		if processed[key] {
			continue
		}

		if classType == "CLIPTextEncode" {
			inputs, ok := value["inputs"].(map[string]any)
			if !ok {
				continue
			}

			var textContentRaw any
			if t, ok := inputs["text"]; ok {
				textContentRaw = t
			} else if p, ok := inputs["prompt"]; ok {
				textContentRaw = p
			}

			if textContentRaw == nil {
				continue
			}

			textContent := ""
			switch v := textContentRaw.(type) {
			case string:
				textContent = v
			case []any:
				var sb strings.Builder
				for i, item := range v {
					if i > 0 {
						sb.WriteString("\n")
					}
					sb.WriteString(fmt.Sprintf("%v", item))
				}
				textContent = sb.String()
			default:
				textContent = fmt.Sprintf("%v", v)
			}

			if strings.TrimSpace(textContent) != "" {
				textContentLower := strings.ToLower(textContent)
				isNegative := strings.Contains(textContentLower[:min(50, len(textContentLower))], "negative")

				if !isNegative {
					positivePrompts = append(positivePrompts, PromptInfo{
						Text:     textContent,
						NodeID:   fmt.Sprintf("%v", key),
						NodeType: classType,
						Title:    fmt.Sprintf("Node %v", key),
						Source:   "prompt_data",
					})
					processed[key] = true
				}
			}
		}
	}

	return positivePrompts
}

func (e *PromptExtractor) extractPositiveFromPNGProperties(meta map[string]string) (string, bool) {
	possibleKeys := []string{
		"Positive prompt",
		"positive prompt",
		"Positive Prompt",
		"positive_prompt",
		"prompt",
		"Prompt",
		"Description",
		"description",
		"Comment",
		"comment",
		"user_comment",
		"UserComment",
	}

	for _, key := range possibleKeys {
		for k, val := range meta {
			if strings.EqualFold(k, key) {
				val = strings.TrimSpace(val)
				val = strings.Trim(val, "\x00\r")
				if val != "" {
					if (strings.HasPrefix(val, "\"") && strings.HasSuffix(val, "\"")) ||
						(strings.HasPrefix(val, "'") && strings.HasSuffix(val, "'")) {
						val = val[1 : len(val)-1]
					}
					return val, true
				}
			}
		}
	}
	return "", false
}

func isDelimiterLine(line string) bool {
	l := strings.ToLower(strings.TrimSpace(line))
	if l == "" {
		return false
	}
	if strings.HasPrefix(l, "negative prompt") || strings.HasPrefix(l, "steps:") {
		return true
	}
	if strings.HasPrefix(l, "ti hashes:") || strings.HasPrefix(l, "lora hashes:") || strings.HasPrefix(l, "hashes:") || strings.HasPrefix(l, "version:") || strings.HasPrefix(l, "template:") {
		return true
	}
	if strings.Contains(l, "steps:") && (strings.Contains(l, "sampler:") || strings.Contains(l, "cfg scale:") || strings.Contains(l, "seed:")) {
		return true
	}
	return false
}

func (e *PromptExtractor) extractPositiveFromParametersStrict(meta map[string]string) (string, bool) {
	var params string
	var found bool
	for k, v := range meta {
		if strings.EqualFold(k, "parameters") {
			params = v
			found = true
			break
		}
	}
	if !found {
		for k, v := range meta {
			if strings.EqualFold(k, "prompt") || strings.EqualFold(k, "description") || strings.EqualFold(k, "comment") {
				params = v
				found = true
				break
			}
		}
	}
	if !found || strings.TrimSpace(params) == "" {
		return "", false
	}

	params = strings.Trim(params, "\x00\r")

	// Try JSON first
	var parsed map[string]any
	if err := json.Unmarshal([]byte(params), &parsed); err == nil {
		possibleKeys := []string{
			"Positive prompt",
			"positive prompt",
			"Positive Prompt",
			"positive_prompt",
			"positive",
			"Positive",
			"prompt",
			"Prompt",
			"text",
			"Text",
		}
		for _, key := range possibleKeys {
			if v, ok := parsed[key]; ok {
				if list, ok := v.([]any); ok {
					var sb strings.Builder
					for i, item := range list {
						if i > 0 {
							sb.WriteString("\n")
						}
						sb.WriteString(fmt.Sprintf("%v", item))
					}
					return sb.String(), true
				}
				if strVal := fmt.Sprintf("%v", v); strings.TrimSpace(strVal) != "" {
					return strVal, true
				}
			}
		}
	}

	// Parse text format (Automatic1111)
	lines := strings.Split(params, "\n")

	// Case A: Look for explicit "Positive prompt:" header
	for i, line := range lines {
		lineTrimmedLower := strings.ToLower(strings.TrimSpace(line))
		if strings.HasPrefix(lineTrimmedLower, "positive prompt:") {
			parts := strings.SplitN(line, ":", 2)
			promptText := ""
			if len(parts) > 1 {
				promptText = strings.TrimSpace(parts[1])
			}

			var promptLines []string
			if promptText != "" {
				promptLines = append(promptLines, promptText)
			}

			j := i + 1
			for j < len(lines) {
				nextLine := lines[j]
				if isDelimiterLine(nextLine) {
					break
				}
				promptLines = append(promptLines, strings.TrimRight(nextLine, "\r\n"))
				j++
			}

			fullPrompt := strings.TrimSpace(strings.Join(promptLines, "\n"))
			if fullPrompt != "" {
				return fullPrompt, true
			}
		}
	}

	// Case B: Standard Automatic1111 format without "Positive prompt:" header
	var promptLines []string
	for _, line := range lines {
		if isDelimiterLine(line) {
			break
		}
		promptLines = append(promptLines, strings.TrimRight(line, "\r\n"))
	}

	fullPrompt := strings.TrimSpace(strings.Join(promptLines, "\n"))
	if fullPrompt != "" {
		return fullPrompt, true
	}

	return "", false
}

// ReadRawMetadata returns the raw metadata chunks/content for a file.
func (e *PromptExtractor) ReadRawMetadata(filePath string) (map[string]string, error) {
	ext := strings.ToLower(filepath.Ext(filePath))
	switch ext {
	case ".png":
		return png.ReadTextChunks(filePath)
	case ".json":
		data, err := os.ReadFile(filePath)
		if err != nil {
			return nil, err
		}
		return map[string]string{"json": string(data)}, nil
	case ".txt":
		data, err := os.ReadFile(filePath)
		if err != nil {
			return nil, err
		}
		return map[string]string{"parameters": string(data)}, nil
	default:
		return nil, fmt.Errorf("unsupported file extension: %s", ext)
	}
}

// CalculateAspectRatio computes a human-readable aspect ratio (e.g. "16:9", "1:1")
// from width and height, matching common photographic/generation ratios within 0.05
// tolerance or falling back to exact GCD reduction.
func CalculateAspectRatio(w, h int) string {
	if h == 0 || w == 0 {
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
	minDiff := 10.0

	for _, r := range ratios {
		diff := math.Abs(ratio - r.val)
		if diff < minDiff {
			minDiff = diff
			bestMatch = r.label
		}
	}

	if minDiff > 0.05 {
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

// FormatPrompts formats extracted positive prompts identically to the Komfy GUI.
// If quiet is true, prompt headers and separators are omitted.
func FormatPrompts(prompts []PromptInfo, quiet bool) string {
	if len(prompts) == 0 {
		return ""
	}
	if quiet {
		var texts []string
		for _, p := range prompts {
			texts = append(texts, strings.TrimSpace(p.Text))
		}
		return strings.Join(texts, "\n\n")
	}

	var promptLines []string
	for j, p := range prompts {
		if len(prompts) > 1 {
			if j > 0 {
				promptLines = append(promptLines, "")
			}
			promptLines = append(promptLines, fmt.Sprintf("Prompt %d - %s:", j+1, p.Title))
			promptLines = append(promptLines, strings.Repeat("-", 40))
		}
		promptLines = append(promptLines, strings.TrimSpace(p.Text))
	}
	return strings.Join(promptLines, "\n")
}

// FormatSummary formats the extraction summary identically to the Komfy GUI Summary tab.
func FormatSummary(result *ExtractionResult, mode string) string {
	var summaryLines []string
	summaryLines = append(summaryLines, "EXTRACTION SUMMARY")
	summaryLines = append(summaryLines, strings.Repeat("-", 20))
	summaryLines = append(summaryLines, "")

	if result == nil {
		return strings.Join(summaryLines, "\n")
	}

	if len(result.PositivePrompts) == 0 && result.Error != "" {
		summaryLines = append(summaryLines, fmt.Sprintf("Error in %s: %s", result.FileInfo.Filename, result.Error))
		return strings.Join(summaryLines, "\n")
	}

	summaryLines = append(summaryLines, fmt.Sprintf("File: %s", result.FileInfo.Filename))
	if mode != "" {
		summaryLines = append(summaryLines, fmt.Sprintf("Mode: %s", mode))
	}
	if result.ExtractionMethod != "" {
		summaryLines = append(summaryLines, fmt.Sprintf("Method: %s", result.ExtractionMethod))
	}
	if result.FileInfo.HasDimensions() {
		summaryLines = append(summaryLines, fmt.Sprintf("Resolution: %dx%d", result.FileInfo.Width, result.FileInfo.Height))
		summaryLines = append(summaryLines, fmt.Sprintf("Aspect Ratio: %s", result.FileInfo.AspectRatio))
	}
	summaryLines = append(summaryLines, fmt.Sprintf("Prompts found: %d", len(result.PositivePrompts)))

	return strings.Join(summaryLines, "\n")
}

// WriteExtractionReport writes the formatted prompt extraction report (1:1 with Komfy GUI "Save To File").
func WriteExtractionReport(w io.Writer, filePath string, mode string, result *ExtractionResult) error {
	writer := bufio.NewWriter(w)
	fmt.Fprintln(writer, strings.Repeat("=", 60))
	fmt.Fprintln(writer, "COMFYUI POSITIVE PROMPTS EXTRACTION")
	fmt.Fprintln(writer, strings.Repeat("=", 60))
	fmt.Fprintf(writer, "\nExtractor mode: %s\n", mode)
	fmt.Fprintf(writer, "File processed: %s\n", filePath)
	if result != nil && result.FileInfo.HasDimensions() {
		fmt.Fprintf(writer, "Resolution: %dx%d\n", result.FileInfo.Width, result.FileInfo.Height)
		fmt.Fprintf(writer, "Aspect ratio: %s\n", result.FileInfo.AspectRatio)
	}
	totalPrompts := 0
	if result != nil {
		totalPrompts = len(result.PositivePrompts)
	}
	fmt.Fprintf(writer, "Total prompts: %d\n", totalPrompts)
	fmt.Fprintf(writer, "Extraction date: %s\n", time.Now().Format("2006-01-02 15:04:05"))
	fmt.Fprintln(writer, "\n"+strings.Repeat("=", 60))

	if result != nil {
		for j, p := range result.PositivePrompts {
			if len(result.PositivePrompts) > 1 {
				fmt.Fprintf(writer, "Prompt %d - %s:\n%s\n", j+1, p.Title, strings.Repeat("-", 40))
			}
			fmt.Fprintln(writer, p.Text)
		}
	}
	return writer.Flush()
}
