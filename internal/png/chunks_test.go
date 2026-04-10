package png

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"os"
	"testing"
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
	binary.Write(f, binary.BigEndian, uint32(0)) // fake CRC
}

func TestReadTextChunks(t *testing.T) {
	// 1. tEXt
	textData := append([]byte("Software"), 0)
	textData = append(textData, []byte("goKomfy")...)

	// 2. zTXt
	var zBuf bytes.Buffer
	zw := zlib.NewWriter(&zBuf)
	zw.Write([]byte("zlib-value"))
	zw.Close()
	zData := append([]byte("Compressed"), 0)
	zData = append(zData, 0) // compression method
	zData = append(zData, zBuf.Bytes()...)

	// 3. iTXt
	iData := append([]byte("UTF8Key"), 0)
	iData = append(iData, 1) // compression flag
	iData = append(iData, 0) // compression method
	iData = append(iData, 0) // language tag (empty)
	iData = append(iData, 0) // translated keyword (empty)
	var ziBuf bytes.Buffer
	ziw := zlib.NewWriter(&ziBuf)
	ziw.Write([]byte("iTXt-value"))
	ziw.Close()
	iData = append(iData, ziBuf.Bytes()...)

	path := createTestPNG(t, []struct {
		typeName string
		data     []byte
	}{
		{"tEXt", textData},
		{"zTXt", zData},
		{"iTXt", iData},
	})
	defer os.Remove(path)

	meta, err := ReadTextChunks(path)
	if err != nil {
		t.Fatal(err)
	}

	if meta["Software"] != "goKomfy" {
		t.Errorf("expected Software=goKomfy, got %s", meta["Software"])
	}
	if meta["Compressed"] != "zlib-value" {
		t.Errorf("expected Compressed=zlib-value, got %s", meta["Compressed"])
	}
	if meta["UTF8Key"] != "iTXt-value" {
		t.Errorf("expected UTF8Key=iTXt-value, got %s", meta["UTF8Key"])
	}
}
