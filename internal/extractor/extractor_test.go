package extractor

import (
	"encoding/binary"
	"hash/crc32"
	"os"
	"testing"
)

var (
	pngSignature = []byte("\x89PNG\r\n\x1a\n")
)

func createTestPNG(t *testing.T, chunks []struct {
	typeName string
	data     []byte
}) string {
	f, err := os.CreateTemp("", "test*.png")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	// Write signature
	f.Write(pngSignature)

	// Write IHDR
	ihdrData := make([]byte, 13)
	binary.BigEndian.PutUint32(ihdrData[0:4], 1)  // width
	binary.BigEndian.PutUint32(ihdrData[4:8], 1)  // height
	ihdrData[8] = 8                               // bit depth
	ihdrData[9] = 2                               // color type (Truecolor)
	ihdrData[10] = 0                              // compression method
	ihdrData[11] = 0                              // filter method
	ihdrData[12] = 0                              // interlace method
	writeChunk(f, "IHDR", ihdrData)

	for _, c := range chunks {
		writeChunk(f, c.typeName, c.data)
	}

	// Write IDAT (minimal)
	writeChunk(f, "IDAT", []byte{0x78, 0x9c, 0x63, 0x00, 0x01, 0x00, 0x01, 0x00, 0x05, 0x00, 0x05})

	// Write IEND
	writeChunk(f, "IEND", nil)

	return f.Name()
}

func writeChunk(f *os.File, typeName string, data []byte) {
	binary.Write(f, binary.BigEndian, uint32(len(data)))
	f.WriteString(typeName)
	f.Write(data)

	h := crc32.NewIEEE()
	h.Write([]byte(typeName))
	h.Write(data)
	binary.Write(f, binary.BigEndian, h.Sum32())
}

func TestExtractComfyUIWorkflow(t *testing.T) {
	workflowJSON := `{
		"nodes": [
			{
				"id": 1,
				"type": "CLIPTextEncode",
				"title": "Positive Prompt",
				"widgets_values": ["masterpiece, best quality, girl"]
			},
			{
				"id": 2,
				"type": "CLIPTextEncode",
				"title": "Negative Prompt",
				"widgets_values": ["low quality, bad anatomy"]
			}
		]
	}`
	textData := append([]byte("workflow"), 0)
	textData = append(textData, []byte(workflowJSON)...)

	path := createTestPNG(t, []struct {
		typeName string
		data     []byte
	}{
		{"tEXt", textData},
	})
	defer os.Remove(path)

	e := &PromptExtractor{}
	result, err := e.ExtractComfyUI(path)
	if err != nil {
		t.Fatal(err)
	}

	if len(result.PositivePrompts) != 1 {
		t.Fatalf("expected 1 positive prompt, got %d", len(result.PositivePrompts))
	}
	if result.PositivePrompts[0].Text != "masterpiece, best quality, girl" {
		t.Errorf("wrong prompt text: %s", result.PositivePrompts[0].Text)
	}
}

func TestExtractComfyUIPrompt(t *testing.T) {
	promptJSON := `{
		"6": {
			"class_type": "CLIPTextEncode",
			"inputs": {
				"text": "beautiful landscape"
			}
		}
	}`
	textData := append([]byte("prompt"), 0)
	textData = append(textData, []byte(promptJSON)...)

	path := createTestPNG(t, []struct {
		typeName string
		data     []byte
	}{
		{"tEXt", textData},
	})
	defer os.Remove(path)

	e := &PromptExtractor{}
	result, err := e.ExtractComfyUI(path)
	if err != nil {
		t.Fatal(err)
	}

	if len(result.PositivePrompts) != 1 {
		t.Fatalf("expected 1 positive prompt, got %d", len(result.PositivePrompts))
	}
	if result.PositivePrompts[0].Text != "beautiful landscape" {
		t.Errorf("wrong prompt text: %s", result.PositivePrompts[0].Text)
	}
}

func TestExtractParameters(t *testing.T) {
	params := "Positive prompt: a cat in a hat\nNegative prompt: dog\nSteps: 20"
	textData := append([]byte("parameters"), 0)
	textData = append(textData, []byte(params)...)

	path := createTestPNG(t, []struct {
		typeName string
		data     []byte
	}{
		{"tEXt", textData},
	})
	defer os.Remove(path)

	e := &PromptExtractor{}
	result, err := e.ExtractParameters(path)
	if err != nil {
		t.Fatal(err)
	}

	if len(result.PositivePrompts) != 1 {
		t.Fatalf("expected 1 positive prompt, got %d", len(result.PositivePrompts))
	}
	if result.PositivePrompts[0].Text != "a cat in a hat" {
		t.Errorf("wrong prompt text: %s", result.PositivePrompts[0].Text)
	}
}

func TestExtractJSON(t *testing.T) {
	workflowJSON := `{
		"nodes": [
			{
				"id": 1,
				"type": "CLIPTextEncode",
				"title": "positive prompt",
				"widgets_values": ["cyberpunk city"]
			}
		]
	}`
	f, err := os.CreateTemp("", "test*.json")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())
	f.Write([]byte(workflowJSON))
	f.Close()

	e := &PromptExtractor{}
	result, err := e.ExtractJSON(f.Name())
	if err != nil {
		t.Fatal(err)
	}

	if len(result.PositivePrompts) != 1 {
		t.Fatalf("expected 1 positive prompt, got %d", len(result.PositivePrompts))
	}
	if result.PositivePrompts[0].Text != "cyberpunk city" {
		t.Errorf("wrong prompt text: %s", result.PositivePrompts[0].Text)
	}
}
