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
		fmt.Fprintln(os.Stderr, "Usage: komfy-cli <file1> [file2] ...")
		os.Exit(1)
	}

	// Expand globs manually (in case shell didn't expand)
	var files []string
	for _, pattern := range args {
		matches, err := filepath.Glob(pattern)
		if err != nil || len(matches) == 0 {
			if _, statErr := os.Stat(pattern); statErr == nil {
				matches = []string{pattern}
			}
		}
		files = append(files, matches...)
	}

	// Deduplicate
	seen := map[string]bool{}
	var unique []string
	for _, f := range files {
		abs, err := filepath.Abs(f)
		if err != nil {
			abs = f
		}
		if !seen[abs] {
			seen[abs] = true
			unique = append(unique, abs)
		}
	}

	e := &extractor.PromptExtractor{}
	for _, filePath := range unique {
		var result *extractor.ExtractionResult
		var err error

		ext := strings.ToLower(filepath.Ext(filePath))
		switch ext {
		case ".json":
			result, err = e.ExtractJSON(filePath)
		case ".png":
			result, err = e.ExtractComfyUI(filePath)
			if err == nil && len(result.PositivePrompts) == 0 {
				result, err = e.ExtractParameters(filePath)
			}
		case ".txt":
			result, err = e.ExtractText(filePath)
		default:
			continue
		}

		if err != nil {
			fmt.Fprintf(os.Stderr, "Error processing %s: %v\n", filePath, err)
			continue
		}

		if len(result.PositivePrompts) > 0 {
			fmt.Printf("=== %s ===\n", filepath.Base(filePath))
			for _, p := range result.PositivePrompts {
				if p.Title != "" && p.Title != "Untitled" {
					fmt.Printf("[%s]\n", p.Title)
				}
				fmt.Println(strings.TrimSpace(p.Text))
				fmt.Println(strings.Repeat("-", 40))
			}
			fmt.Println()
		}
	}
}
