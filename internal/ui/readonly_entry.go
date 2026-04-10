package ui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/widget"
)

// ReadOnlyEntry is a widget that looks like an entry but doesn't allow editing.
// This is better than Disable() because it keeps the theme's text color.
type ReadOnlyEntry struct {
	widget.Entry
}

func NewReadOnlyEntry() *ReadOnlyEntry {
	e := &ReadOnlyEntry{}
	e.MultiLine = true
	e.ExtendBaseWidget(e)
	return e
}

func (e *ReadOnlyEntry) TypedRune(r rune)            {}
func (e *ReadOnlyEntry) TypedKey(k *fyne.KeyEvent)   {}
func (e *ReadOnlyEntry) Paste()                      {}
func (e *ReadOnlyEntry) Cut()                        {}
func (e *ReadOnlyEntry) DoubleTapped(p *fyne.PointEvent) {
	e.Entry.DoubleTapped(p)
}
