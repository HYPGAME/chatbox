//go:build !darwin

package tui

func defaultClipboardWriter() clipboardWriterFunc {
	return func(string) error {
		return errClipboardUnsupported
	}
}
