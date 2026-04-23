package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mamorett/goKomfy/internal/extractor"
)

func main() {
	flag.Parse()
	args := flag.Args()
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "Usage: komfy-cli <file>")
		os.Exit(1)
	}

	filePath := args[0]
	e := &extractor.PromptExtractor{}
	var result *extractor.ExtractionResult
	var err error

	ext := strings.ToLower(filepath.Ext(filePath))
	switch ext {
	case ".json":
		result, err = e.ExtractJSON(filePath)
	case ".png":
		result, err = e.ExtractComfyUI(filePath, nil)
		if err == nil && len(result.PositivePrompts) == 0 {
			result, err = e.ExtractParameters(filePath, nil)
		}
	case ".txt":
		result, err = e.ExtractText(filePath)
	default:
		fmt.Fprintf(os.Stderr, "Unsupported file extension: %s\n", ext)
		os.Exit(1)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "Error processing %s: %v\n", filePath, err)
		os.Exit(1)
	}

	if len(result.PositivePrompts) > 0 {
		for _, p := range result.PositivePrompts {
			if p.Title != "" && p.Title != "Untitled" {
				fmt.Printf("[%s]\n", p.Title)
			}
			fmt.Println(strings.TrimSpace(p.Text))
			if len(result.PositivePrompts) > 1 {
				fmt.Println(strings.Repeat("-", 40))
			}
		}
	}
}
