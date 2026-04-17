package extractor

import (
	"encoding/json"
	"fmt"
	"image"
	_ "image/png"
	"os"
	"path/filepath"
	"strings"

	"github.com/mamorett/goKomfy/internal/png"
)

// FileInfo describes the source file.
type FileInfo struct {
	Filename string
	Width    int
	Height   int
	Mode     string // "PNG", "JSON", "TEXT"
}

// PromptInfo holds one extracted positive prompt.
type PromptInfo struct {
	Text     string
	NodeID   string
	NodeType string
	Title    string
	Source   string
}

// ExtractionResult is the result for one file.
type ExtractionResult struct {
	FileInfo         FileInfo
	PositivePrompts  []PromptInfo
	ExtractionMethod string
	Error            string
}

// ExtractionOptions allows passing pre-calculated dimensions.
type ExtractionOptions struct {
	Width  int
	Height int
}

type PromptExtractor struct{}

func (e *PromptExtractor) ExtractComfyUI(filePath string, opts ...*ExtractionOptions) (*ExtractionResult, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var w, h int
	if len(opts) > 0 && opts[0] != nil && opts[0].Width > 0 {
		w = opts[0].Width
		h = opts[0].Height
	} else {
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
			Filename: filepath.Base(filePath),
			Width:    w,
			Height:   h,
			Mode:     "PNG",
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
				// For prompt data, we need a map[string]bool for processed nodes
				// but extractPositiveFromWorkflow uses map[any]bool.
				// Let's make extractPositiveFromPromptData also use map[any]bool for consistency if possible,
				// or just convert.
				prompts := e.extractPositiveFromPromptData(promptData, processedNodes)
				result.PositivePrompts = append(result.PositivePrompts, prompts...)
			}
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
	if len(opts) > 0 && opts[0] != nil && opts[0].Width > 0 {
		w = opts[0].Width
		h = opts[0].Height
	} else {
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
			Filename: filepath.Base(filePath),
			Width:    w,
			Height:   h,
			Mode:     "PNG",
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
	} else {
		// If original method fails, try PNG properties as fallback
		if promptText, ok := e.extractPositiveFromPNGProperties(meta); ok {
			result.PositivePrompts = append(result.PositivePrompts, PromptInfo{
				Text:     promptText,
				NodeID:   "png_properties",
				NodeType: "png_properties",
				Title:    "PNG Properties",
				Source:   "png_properties",
			})
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
	}

	for _, key := range possibleKeys {
		if val, ok := meta[key]; ok {
			val = strings.TrimSpace(val)
			if val != "" {
				if (strings.HasPrefix(val, "\"") && strings.HasSuffix(val, "\"")) ||
					(strings.HasPrefix(val, "'") && strings.HasSuffix(val, "'")) {
					val = val[1 : len(val)-1]
				}
				return val, true
			}
		}
	}
	return "", false
}

func (e *PromptExtractor) extractPositiveFromParametersStrict(meta map[string]string) (string, bool) {
	params, ok := meta["parameters"]
	if !ok {
		return "", false
	}

	// Try JSON first
	var parsed map[string]any
	if err := json.Unmarshal([]byte(params), &parsed); err == nil {
		possibleKeys := []string{
			"Positive prompt",
			"positive prompt",
			"Positive Prompt",
			"positive_prompt",
			"prompt",
			"Prompt",
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
				return fmt.Sprintf("%v", v), true
			}
		}
	}

	// Parse text format
	lines := strings.Split(params, "\n")
	for i, line := range lines {
		lineTrimmedLower := strings.ToLower(strings.TrimSpace(line))
		if strings.HasPrefix(lineTrimmedLower, "positive prompt:") {
			parts := strings.SplitN(line, ":", 2)
			promptText := ""
			if len(parts) > 1 {
				promptText = strings.TrimSpace(parts[1])
			}

			promptLines := []string{}
			if promptText != "" {
				promptLines = append(promptLines, promptText)
			}

			j := i + 1
			for j < len(lines) {
				nextLine := lines[j]
				nl := strings.ToLower(strings.TrimSpace(nextLine))
				if strings.Contains(nl, ":") {
					foundParam := false
					for _, param := range []string{"negative prompt", "steps", "sampler", "cfg scale", "seed", "size", "model", "clip skip"} {
						if strings.Contains(nl, param) {
							foundParam = true
							break
						}
					}
					if foundParam {
						break
					}
				}
				promptLines = append(promptLines, strings.TrimRight(nextLine, "\r\n"))
				j++
			}

			fullPrompt := strings.TrimRight(strings.Join(promptLines, "\n"), "\n\r ")
			outLines := strings.Split(fullPrompt, "\n")
			k := 0
			for k < len(outLines) && strings.TrimSpace(outLines[k]) == "" {
				k++
			}
			if k < len(outLines) {
				return strings.Join(outLines[k:], "\n"), true
			}
			return "", false
		}
	}

	return "", false
}
