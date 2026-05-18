package ui

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
)

type komfyTheme struct{}

func (m *komfyTheme) Color(name fyne.ThemeColorName, variant fyne.ThemeVariant) color.Color {
	// Nord Palette implementation
	if variant == theme.VariantLight {
		return theme.DefaultTheme().Color(name, variant)
	}

	switch name {
	case theme.ColorNameBackground:
		return color.RGBA{R: 0x2e, G: 0x34, B: 0x40, A: 0xff} // nord0
	case theme.ColorNameForeground:
		return color.RGBA{R: 0xd8, G: 0xde, B: 0xe9, A: 0xff} // nord4
	case theme.ColorNamePrimary, theme.ColorNameFocus:
		return color.RGBA{R: 0x88, G: 0xc0, B: 0xd0, A: 0xff} // nord8 (Frost Blue)
	case theme.ColorNameButton:
		return color.RGBA{R: 0x3b, G: 0x42, B: 0x52, A: 0xff} // nord1
	case theme.ColorNameHover:
		return color.RGBA{R: 0x43, G: 0x4c, B: 0x5e, A: 0xff} // nord2
	case theme.ColorNameInputBackground:
		return color.RGBA{R: 0x3b, G: 0x42, B: 0x52, A: 0xff} // nord1
	case theme.ColorNameScrollBar:
		return color.RGBA{R: 0x4c, G: 0x56, B: 0x6a, A: 0x99} // nord3
	case theme.ColorNameSeparator:
		return color.RGBA{R: 0x3b, G: 0x42, B: 0x52, A: 0xff} // nord1
	case theme.ColorNameSuccess:
		return color.RGBA{R: 0xa3, G: 0xbe, B: 0x8c, A: 0xff} // nord14 (Aurora Green)
	case theme.ColorNameWarning:
		return color.RGBA{R: 0xeb, G: 0xcb, B: 0x8b, A: 0xff} // nord13 (Aurora Yellow)
	case theme.ColorNameError:
		return color.RGBA{R: 0xbf, G: 0x61, B: 0x6a, A: 0xff} // nord11 (Aurora Red)
	}

	return theme.DefaultTheme().Color(name, variant)
}

func (m *komfyTheme) Font(style fyne.TextStyle) fyne.Resource {
	return theme.DefaultTheme().Font(style)
}

func (m *komfyTheme) Icon(name fyne.ThemeIconName) fyne.Resource {
	return theme.DefaultTheme().Icon(name)
}

func (m *komfyTheme) Size(name fyne.ThemeSizeName) float32 {
	if name == theme.SizeNamePadding {
		return 6 // Slightly tighter padding
	}
	if name == theme.SizeNameInnerPadding {
		return 4
	}
	if name == theme.SizeNameText {
		return 13
	}
	return theme.DefaultTheme().Size(name)
}

func NewKomfyTheme() fyne.Theme {
	return &komfyTheme{}
}
