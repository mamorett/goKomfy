<div align="center">
  <img src="cmd/komfy/logo.png" width="200" height="200" alt="goKomfy Logo">
  <h1>goKomfy</h1>
  <p><strong>A high-performance, cross-platform ComfyUI Prompt Extractor</strong></p>
  <p>
    <img src="https://img.shields.io/badge/Go-00ADD8?style=for-the-badge&logo=go&logoColor=white" alt="Go">
    <img src="https://img.shields.io/badge/Fyne-2D3E50?style=for-the-badge&logo=fyne&logoColor=white" alt="Fyne">
    <img src="https://img.shields.io/badge/License-MIT-green?style=for-the-badge" alt="License">
  </p>
</div>

---

**goKomfy** is a fast and lightweight tool designed to extract positive prompts from **ComfyUI-generated PNG files** and **JSON workflows**. 

Ported from the original Python/PyQt6 application to Go, it offers superior performance, a refined user interface, and single-binary distribution.

## ✨ Key Features

- 📂 **Dual Extraction Modes**:
  - **ComfyUI**: Native extraction from `workflow` and `prompt` metadata chunks.
  - **Parameters**: Support for standard `parameters` metadata (A1111/Forge style).
- 🖱️ **Enhanced Drag & Drop**:
  - Intuitive, high-visibility drop zone with interactive visual feedback (flashing confirm).
  - Supports dropping single files, multiple files, or entire folders.
- 📋 **Auto-Copy to Clipboard**:
  - Optional switch to automatically copy all extracted prompts as soon as they are processed.
- 🖼️ **Smart Image Preview**:
  - Visual thumbnail for PNG files.
  - **Automatic Aspect Ratio** calculation (16:9, 4:3, 1:1, etc.) displayed alongside image dimensions.
- ⌨️ **Keyboard Optimized**: Shortcuts for all major actions (Mode toggle, Open, Save, Clear).
- 🐚 **Headless CLI**: A dedicated command-line tool for automated workflows and terminal enthusiasts.

## 🚀 Installation

### Pre-built Binaries
Grab the latest release for your platform from the [Releases](https://github.com/mamorett/goKomfy/releases) page.

### Building from Source

**Prerequisites**:
- **Go 1.21+**: [Install Go](https://go.dev/dl/)
- **Fyne CLI**: `go install fyne.io/tools/cmd/fyne@latest` (used for bundling)
- **C Compiler**: GCC or Clang
- **Graphics Headers**:
  - **Ubuntu/Debian**: `sudo apt install build-essential libgl1-mesa-dev xorg-dev`
  - **Fedora**: `sudo dnf groupinstall "Development Tools" "X Software Development" && sudo dnf install mesa-libGL-devel`
  - **Arch**: `sudo pacman -S base-devel xorg-server-devel`
  - **macOS**: `xcode-select --install`
  - **Windows**: [MSYS2](https://www.msys2.org/) with `mingw-w64-x86_64-toolchain`

```bash
# 1. Clone the repository
git clone https://github.com/mamorett/goKomfy.git
cd goKomfy

# 2. Build for host platform (creates binaries in build/)
make

# 3. Install (Linux Only)
# Adds binaries to ~/.local/bin and creates a desktop entry
make install-linux
```

## 📖 Usage

### GUI Application (`gokomfy`)
The main interface for most users. 
- **Toggle Mode**: `Ctrl+E` or use the dropdown.
- **Open Files**: `Ctrl+O` or Drag & Drop.
- **Aspect Ratio**: Automatically shown below the preview image.
- **Auto-Copy**: Enable the checkbox to skip manual copying.

### CLI Tool (`gokomfy-cli`)
Perfect for scripts or quick terminal checks.
```bash
./build/gokomfy-cli path/to/image.png
```

## 🛠️ System Requirements (Linux)
- **Clipboard**: Requires `xclip`, `xsel`, or a Wayland clipboard daemon (e.g., `wl-clipboard`).
- **Icons/Desktop**: Desktop integration uses `update-desktop-database` and `gtk-update-icon-cache`.

## 📄 License
MIT License. See [LICENSE](LICENSE) for details.
