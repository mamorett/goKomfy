package ui

import (
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
	onDropped func(paths []string)
	hovered   bool
}

func NewDropZone(text string, onDropped func(paths []string)) *DropZone {
	d := &DropZone{
		label:     widget.NewLabel(text),
		onDropped: onDropped,
	}
	d.label.Alignment = fyne.TextAlignCenter
	d.label.Wrapping = fyne.TextWrapWord
	d.ExtendBaseWidget(d)
	return d
}

func (d *DropZone) CreateRenderer() fyne.WidgetRenderer {
	bg := canvas.NewRectangle(color.Transparent)
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
	r.dropZone.label.Resize(size)
}

func (r *dropZoneRenderer) MinSize() fyne.Size {
	return r.dropZone.label.MinSize().Max(fyne.NewSize(200, 100))
}

func (r *dropZoneRenderer) Refresh() {
	if r.dropZone.hovered {
		r.border.StrokeColor = theme.PrimaryColor()
		r.bg.FillColor = theme.HoverColor()
	} else {
		r.border.StrokeColor = theme.DisabledColor()
		r.bg.FillColor = color.Transparent
	}
	r.bg.Refresh()
	r.border.Refresh()
	r.dropZone.label.Refresh()
}

func (r *dropZoneRenderer) Objects() []fyne.CanvasObject {
	return r.objects
}

func (r *dropZoneRenderer) Destroy() {}

// MouseIn is called when a mouse enters the widget.
func (d *DropZone) MouseIn(ev *desktop.MouseEvent) {
	d.hovered = true
	d.Refresh()
}

// MouseOut is called when a mouse exits the widget.
func (d *DropZone) MouseOut() {
	d.hovered = false
	d.Refresh()
}

// Implement URIDropTarget
func (d *DropZone) DroppedURIs(uris []fyne.URI) {
	var paths []string
	for _, u := range uris {
		if u.Scheme() == "file" {
			paths = append(paths, u.Path())
		}
	}
	if d.onDropped != nil && len(paths) > 0 {
		d.onDropped(paths)
	}
}
