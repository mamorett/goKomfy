package main

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"hash/crc32"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var pngSig = []byte("\x89PNG\r\n\x1a\n")

func createTestPNGWithDims(t *testing.T, width, height uint32, chunks map[string]string) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "test*.png")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	_, _ = f.Write(pngSig)

	ihdr := make([]byte, 13)
	binary.BigEndian.PutUint32(ihdr[0:4], width)
	binary.BigEndian.PutUint32(ihdr[4:8], height)
	ihdr[8] = 8
	ihdr[9] = 2
	writePNGChunk(f, "IHDR", ihdr)

	for k, v := range chunks {
		data := append([]byte(k), 0)
		data = append(data, []byte(v)...)
		writePNGChunk(f, "tEXt", data)
	}

	writePNGChunk(f, "IDAT", []byte{0x78, 0x9c, 0x63, 0x00, 0x01, 0x00, 0x01, 0x00, 0x05, 0x00, 0x05})
	writePNGChunk(f, "IEND", nil)

	return f.Name()
}

func writePNGChunk(f *os.File, typeName string, data []byte) {
	_ = binary.Write(f, binary.BigEndian, uint32(len(data)))
	_, _ = f.WriteString(typeName)
	_, _ = f.Write(data)

	h := crc32.NewIEEE()
	_, _ = h.Write([]byte(typeName))
	_, _ = h.Write(data)
	_ = binary.Write(f, binary.BigEndian, h.Sum32())
}

func TestHelpNoArgsAndFlag(t *testing.T) {
	// 1. Run without arguments -> must print detailed help
	var out, errBuf bytes.Buffer
	code := run([]string{}, &out, &errBuf)
	if code != 0 {
		t.Fatalf("expected exit code 0 when run without args, got %d", code)
	}
	helpText := out.String()
	for _, expected := range []string{
		"DESCRIPTION",
		"USAGE",
		"EXTRACTION MODE & PROMPT OPTIONS",
		"IMAGE GEOMETRY OPTIONS",
		"--comfyui",
		"--parameters",
		"--resolution",
		"--ratio",
		"EXAMPLES",
	} {
		if !strings.Contains(helpText, expected) {
			t.Errorf("help output missing %q", expected)
		}
	}

	// 2. Run with --help -> must print identical detailed help
	var out2, errBuf2 bytes.Buffer
	code2 := run([]string{"--help"}, &out2, &errBuf2)
	if code2 != 0 {
		t.Fatalf("expected exit code 0 with --help, got %d", code2)
	}
	if out2.String() != helpText {
		t.Errorf("expected --help output to match no-args help output")
	}
}

func TestComfyUIAndParametersExtraction(t *testing.T) {
	workflowJSON := `{
		"nodes": [
			{
				"id": 1,
				"type": "CLIPTextEncode",
				"title": "Positive Prompt",
				"widgets_values": ["comfyui workflow prompt text"]
			}
		]
	}`
	paramsText := "Positive prompt: a1111 parameter prompt text\nNegative prompt: blurry\nSteps: 25, Sampler: Euler a"

	// Image with BOTH ComfyUI workflow and A1111 parameters
	imgPath := createTestPNGWithDims(t, 1920, 1080, map[string]string{
		"workflow":   workflowJSON,
		"parameters": paramsText,
	})

	// Test --comfyui (-c)
	var out, errBuf bytes.Buffer
	if code := run([]string{"--comfyui", imgPath}, &out, &errBuf); code != 0 {
		t.Fatalf("expected exit code 0, got %d (stderr: %s)", code, errBuf.String())
	}
	if strings.TrimSpace(out.String()) != "comfyui workflow prompt text" {
		t.Errorf("unexpected comfyui output: %q", out.String())
	}

	// Test --parameters (-p)
	out.Reset()
	errBuf.Reset()
	if code := run([]string{"--parameters", imgPath}, &out, &errBuf); code != 0 {
		t.Fatalf("expected exit code 0, got %d (stderr: %s)", code, errBuf.String())
	}
	if strings.TrimSpace(out.String()) != "a1111 parameter prompt text" {
		t.Errorf("unexpected parameters output: %q", out.String())
	}

	// Test --parameter alias
	out.Reset()
	errBuf.Reset()
	if code := run([]string{"--parameter", imgPath}, &out, &errBuf); code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}
	if strings.TrimSpace(out.String()) != "a1111 parameter prompt text" {
		t.Errorf("unexpected --parameter output: %q", out.String())
	}
}

func TestResolutionAndRatioOptions(t *testing.T) {
	workflowJSON := `{
		"nodes": [
			{
				"id": 1,
				"type": "CLIPTextEncode",
				"title": "Positive Prompt",
				"widgets_values": ["cinematic vista"]
			}
		]
	}`
	imgPath := createTestPNGWithDims(t, 1920, 1080, map[string]string{
		"workflow": workflowJSON,
	})

	// 1. --resolution only
	var out, errBuf bytes.Buffer
	if code := run([]string{"--resolution", imgPath}, &out, &errBuf); code != 0 {
		t.Fatalf("expected 0, got %d", code)
	}
	if !strings.Contains(out.String(), "1920x1080") || strings.Contains(out.String(), "cinematic vista") {
		t.Errorf("expected only resolution, got: %q", out.String())
	}

	// 2. --ratio only
	out.Reset()
	if code := run([]string{"--ratio", imgPath}, &out, &errBuf); code != 0 {
		t.Fatalf("expected 0, got %d", code)
	}
	if !strings.Contains(out.String(), "16:9") || strings.Contains(out.String(), "cinematic vista") {
		t.Errorf("expected only ratio 16:9, got: %q", out.String())
	}

	// 3. Combined -c -r -a (prompt + resolution + ratio)
	out.Reset()
	if code := run([]string{"-cra", imgPath}, &out, &errBuf); code != 0 {
		t.Fatalf("expected 0, got %d", code)
	}
	got := out.String()
	if !strings.Contains(got, "1920x1080") || !strings.Contains(got, "16:9") || !strings.Contains(got, "cinematic vista") {
		t.Errorf("expected resolution, ratio, and prompt in output, got:\n%s", got)
	}

	// 4. Quiet mode -q -r -a
	out.Reset()
	if code := run([]string{"-q", "-r", "-a", imgPath}, &out, &errBuf); code != 0 {
		t.Fatalf("expected 0, got %d", code)
	}
	if strings.TrimSpace(out.String()) != "1920x1080\n16:9" {
		t.Errorf("unexpected quiet output: %q", out.String())
	}
}

func TestSaveAndJSONOutput(t *testing.T) {
	workflowJSON := `{
		"nodes": [
			{
				"id": 1,
				"type": "CLIPTextEncode",
				"title": "Positive Prompt",
				"widgets_values": ["neon samurai"]
			}
		]
	}`
	imgPath := createTestPNGWithDims(t, 1024, 1024, map[string]string{
		"workflow": workflowJSON,
	})

	reportPath := filepath.Join(t.TempDir(), "saved_report.txt")
	var out, errBuf bytes.Buffer
	if code := run([]string{"-j", "-o", reportPath, imgPath}, &out, &errBuf); code != 0 {
		t.Fatalf("expected 0, got %d (stderr: %s)", code, errBuf.String())
	}

	// Check JSON output
	var parsed jsonFileOutput
	if err := json.Unmarshal(out.Bytes(), &parsed); err != nil {
		t.Fatalf("invalid JSON output: %v\n%s", err, out.String())
	}
	if parsed.FileInfo.Resolution != "1024x1024" || parsed.FileInfo.AspectRatio != "1:1" {
		t.Errorf("unexpected file_info in JSON: %+v", parsed.FileInfo)
	}
	if len(parsed.PositivePrompts) != 1 || parsed.PositivePrompts[0].Text != "neon samurai" {
		t.Errorf("unexpected prompts in JSON: %+v", parsed.PositivePrompts)
	}

	// Check saved report file
	savedBytes, err := os.ReadFile(reportPath)
	if err != nil {
		t.Fatalf("failed to read saved report: %v", err)
	}
	savedStr := string(savedBytes)
	if !strings.Contains(savedStr, "COMFYUI POSITIVE PROMPTS EXTRACTION") || !strings.Contains(savedStr, "neon samurai") {
		t.Errorf("saved report missing expected content:\n%s", savedStr)
	}
}
