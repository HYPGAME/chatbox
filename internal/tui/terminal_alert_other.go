//go:build !darwin

package tui

type terminalAppForegroundDetector struct {
	currentTTY string
	runScript  func(string) (string, error)
}

func newTerminalAppForegroundDetector(string) terminalAppForegroundDetector {
	return terminalAppForegroundDetector{}
}

func (terminalAppForegroundDetector) ShouldAlert() bool {
	return false
}

func newTerminalBellAlertNotifier(func()) alertNotifierFunc {
	return nil
}
