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
	label     *widget.Label
	hovered   bool
	blinking  bool
}

func NewDropZone(text string) *DropZone {
	d := &DropZone{
		label: widget.NewLabel(text),
	}
	d.label.Alignment = fyne.TextAlignCenter
	d.label.Wrapping = fyne.TextWrapWord
	d.ExtendBaseWidget(d)
	return d
}

func (d *DropZone) CreateRenderer() fyne.WidgetRenderer {
	bg := canvas.NewRectangle(theme.InputBackgroundColor())
	
	// Use a slightly visible border
	border := canvas.NewRectangle(color.Transparent)
	border.StrokeWidth = 2
	border.StrokeColor = theme.DisabledColor()

	return &dropZoneRenderer{
		dropZone: d,
		bg:       bg,
		border:   border,
		objects:  []fyne.CanvasObject{bg, border, d.label},
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
	// Center the label with some padding
	padding := fyne.NewSize(20, 20)
	r.dropZone.label.Resize(size.Subtract(padding))
	r.dropZone.label.Move(fyne.NewPos(10, 10))
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
		r.bg.FillColor = theme.HoverColor()
		r.dropZone.label.TextStyle.Bold = false
	} else {
		r.border.StrokeColor = theme.DisabledColor()
		r.bg.FillColor = theme.InputBackgroundColor()
		r.dropZone.label.TextStyle.Bold = false
	}
	r.bg.Refresh()
	r.border.Refresh()
	r.dropZone.label.Refresh()
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
