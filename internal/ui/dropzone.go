package ui

import (
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"image/color"
)

type DropZone struct {
	widget.BaseWidget
	label    *widget.Label
	icon     *widget.Icon
	hovered  bool
	blinking bool
}

func NewDropZone(text string) *DropZone {
	d := &DropZone{
		label: widget.NewLabel(text),
		icon:  widget.NewIcon(theme.UploadIcon()),
	}
	d.label.Alignment = fyne.TextAlignCenter
	d.label.Wrapping = fyne.TextWrapWord
	d.ExtendBaseWidget(d)
	return d
}

func (d *DropZone) CreateRenderer() fyne.WidgetRenderer {
	bg := canvas.NewRectangle(theme.InputBackgroundColor())
	bg.CornerRadius = 12

	border := canvas.NewRectangle(color.Transparent)
	border.StrokeWidth = 2
	border.StrokeColor = theme.DisabledColor()

	return &dropZoneRenderer{
		dropZone: d,
		bg:       bg,
		border:   border,
		objects:  []fyne.CanvasObject{bg, border, d.icon, d.label},
	}
}

type dropZoneRenderer struct {
	dropZone *DropZone
	bg       *canvas.Rectangle
	border   *canvas.Rectangle
	objects  []fyne.CanvasObject
}

func (r *dropZoneRenderer) Layout(size fyne.Size) {
	r.bg.Resize(size)
	r.border.Resize(size)

	iconSize := float32(48)
	r.dropZone.icon.Resize(fyne.NewSize(iconSize, iconSize))
	r.dropZone.icon.Move(fyne.NewPos((size.Width-iconSize)/2, (size.Height-iconSize)/2-20))

	labelWidth := size.Width - 40
	r.dropZone.label.Resize(fyne.NewSize(labelWidth, 40))
	r.dropZone.label.Move(fyne.NewPos(20, (size.Height-iconSize)/2+iconSize-10))
}

func (r *dropZoneRenderer) MinSize() fyne.Size {
	return fyne.NewSize(150, 100)
}

func (r *dropZoneRenderer) Refresh() {
	if r.dropZone.blinking {
		r.bg.FillColor = theme.PrimaryColor()
		r.border.StrokeColor = theme.PrimaryColor()
		r.dropZone.label.TextStyle.Bold = true
	} else if r.dropZone.hovered {
		r.border.StrokeColor = theme.PrimaryColor()
		r.border.StrokeWidth = 3
		r.bg.FillColor = theme.HoverColor()
		r.dropZone.label.TextStyle.Bold = false
	} else {
		r.border.StrokeColor = theme.SeparatorColor()
		r.border.StrokeWidth = 2
		r.bg.FillColor = theme.InputBackgroundColor()
		r.dropZone.label.TextStyle.Bold = false
	}
	r.bg.Refresh()
	r.border.Refresh()
	r.dropZone.label.Refresh()
	r.dropZone.icon.Refresh()
}

func (r *dropZoneRenderer) Objects() []fyne.CanvasObject {
	return r.objects
}

func (r *dropZoneRenderer) Destroy() {}

func (d *DropZone) MouseIn(ev *desktop.MouseEvent) {
	d.hovered = true
	d.Refresh()
}

func (d *DropZone) MouseOut() {
	d.hovered = false
	d.Refresh()
}

func (d *DropZone) Flash() {
	d.blinking = true
	d.Refresh()
	go func() {
		time.Sleep(150 * time.Millisecond)
		fyne.Do(func() {
			d.blinking = false
			d.Refresh()
		})
	}()
}
