# goKomfy — ComfyUI Prompt Extractor (Go Edition)

A high-performance, cross-platform tool to extract positive prompts from ComfyUI-generated PNG files and JSON workflows. Ported from the original Python/PyQt6 application to Go for better performance and a single-binary distribution.

## Features

- **Dual Extraction Modes**:
  - **ComfyUI**: Extracts prompts from internal `workflow` and `prompt` metadata chunks.
  - **Parameters**: Extracts prompts from standard `parameters` metadata (A1111/Forge style).
- **Batch Processing**: Select multiple files or entire folders at once.
- **Drag & Drop**: Intuitive GUI interface using the Fyne toolkit.
- **Image Thumbnails**: Visual preview for single PNG files.
- **CLI Tool**: Headless version for automated workflows and terminal use.
- **Native Binaries**: Runs on Linux (AMD64/ARM64) and macOS (Apple Silicon).

## Installation

### Pre-built Binaries
Download the latest binary for your platform from the Releases page.

### Building from Source
**Prerequisites**:
- Go 1.21 or higher
- A C compiler (GCC or Clang) for GUI builds (CGo)
- OpenGL development headers:
  - **Linux (X11)**: `sudo apt install libgl1-mesa-dev xorg-dev`
  - **macOS**: Xcode Command Line Tools (`xcode-select --install`)

**Build commands**:
```bash
# Build both GUI and CLI for host platform
make

# Build for specific platforms
make linux-amd64
make linux-arm64
make macos-arm64
```

## Usage

### GUI Application (`komfy`)
Run the binary:
```bash
./build/komfy-darwin-arm64  # macOS
./build/komfy-linux-amd64   # Linux
```
- Drag and drop files/folders into the window.
- Use **Ctrl+E** to toggle between ComfyUI and Parameters extraction modes.
- Copy all or specific prompts to the clipboard.
- Save extracted prompts to a `.txt` file.

### CLI Tool (`komfy-cli`)
```bash
./build/komfy-cli <file1> <file2> <pattern/*.png>
```
The CLI tool will print extracted prompts for each valid file to standard output.

## Runtime Requirements (Linux Only)
The clipboard functionality requires one of the following to be installed:
- `xclip`
- `xsel`
- A Wayland clipboard daemon (if using Wayland)

## Support
Supported file formats:
- **ComfyUI PNG**: PNGs with `workflow` or `prompt` metadata chunks.
- **Parameters PNG**: PNGs with `parameters` metadata (Automatic1111 style).
- **JSON Workflow**: ComfyUI workflow files (exported from the UI).

## License
MIT License. See `LICENSE` for details.
