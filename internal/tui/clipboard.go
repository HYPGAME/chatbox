package tui

import "errors"

type clipboardWriterFunc func(string) error

var errClipboardUnsupported = errors.New("copy unsupported")
