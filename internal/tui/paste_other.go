//go:build !darwin

package tui

import "context"

func defaultClipboardReader() clipboardReaderFunc {
	return func(context.Context) (clipboardAttachment, error) {
		return clipboardAttachment{}, errPasteUnsupported
	}
}
