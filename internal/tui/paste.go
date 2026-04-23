package tui

import (
	"context"
	"errors"
)

type clipboardAttachment struct {
	Path    string
	Kind    string
	Cleanup func()
}

type clipboardReaderFunc func(context.Context) (clipboardAttachment, error)

var (
	errPasteUnsupported = errors.New("paste unsupported")
	errPasteEmpty       = errors.New("clipboard does not contain an image or file")
)
