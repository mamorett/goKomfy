// Package png provides a raw PNG chunk reader for metadata extraction.
package png

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"errors"
	"io"
	"os"
)

var (
	pngSignature = []byte("\x89PNG\r\n\x1a\n")
)

// ReadTextChunks opens a PNG file and returns all text metadata as a map.
// Keys are chunk keywords (e.g. "workflow", "prompt", "parameters",
// "Positive prompt"). Values are the decoded string content.
func ReadTextChunks(filePath string) (map[string]string, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	// 1. Verify PNG signature
	sig := make([]byte, 8)
	if _, err := io.ReadFull(f, sig); err != nil {
		return nil, err
	}
	if !bytes.Equal(sig, pngSignature) {
		return nil, errors.New("not a valid PNG file")
	}

	meta := make(map[string]string)

	// 2. Loop reading chunks
	for {
		var length uint32
		if err := binary.Read(f, binary.BigEndian, &length); err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}

		chunkType := make([]byte, 4)
		if _, err := io.ReadFull(f, chunkType); err != nil {
			return nil, err
		}

		data := make([]byte, length)
		if _, err := io.ReadFull(f, data); err != nil {
			return nil, err
		}

		// Read CRC (ignore for now)
		var crc uint32
		if err := binary.Read(f, binary.BigEndian, &crc); err != nil {
			return nil, err
		}

		typeName := string(chunkType)
		if typeName == "IEND" {
			break
		}

		switch typeName {
		case "tEXt":
			// 3. tEXt: split at first \x00
			parts := bytes.SplitN(data, []byte{0}, 2)
			if len(parts) == 2 {
				key := string(parts[0])
				// Latin-1 to UTF-8 conversion: treat each byte as its Unicode codepoint
				val := string(parts[1])
				meta[key] = val
			}
		case "zTXt":
			// 4. zTXt: split at first \x00 for key; next byte is compression method (must be 0); remainder is zlib
			parts := bytes.SplitN(data, []byte{0}, 2)
			if len(parts) == 2 {
				key := string(parts[0])
				if len(parts[1]) > 0 && parts[1][0] == 0 {
					zlibData := parts[1][1:]
					zr, err := zlib.NewReader(bytes.NewReader(zlibData))
					if err == nil {
						var buf bytes.Buffer
						if _, err := io.Copy(&buf, zr); err == nil {
							meta[key] = buf.String()
						}
						zr.Close()
					}
				}
			}
		case "iTXt":
			// 5. iTXt: keyword\0compressionFlag\0compressionMethod\0languageTag\0translatedKeyword\0text
			parts := bytes.SplitN(data, []byte{0}, 2)
			if len(parts) == 2 {
				key := string(parts[0])
				rest := parts[1]
				if len(rest) >= 2 {
					compFlag := rest[0]
					// rest[1] is compressionMethod
					subParts := bytes.SplitN(rest[2:], []byte{0}, 3)
					if len(subParts) == 3 {
						// subParts[0] is languageTag
						// subParts[1] is translatedKeyword
						valData := subParts[2]
						if compFlag == 1 {
							zr, err := zlib.NewReader(bytes.NewReader(valData))
							if err == nil {
								var buf bytes.Buffer
								if _, err := io.Copy(&buf, zr); err == nil {
									meta[key] = buf.String()
								}
								zr.Close()
							}
						} else {
							meta[key] = string(valData)
						}
					}
				}
			}
		}
	}

	return meta, nil
}
