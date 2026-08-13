//go:build windows

package launch

import "errors"

type unsupportedWindowController struct{}

func newWindowController() WindowController { return unsupportedWindowController{} }
func (unsupportedWindowController) Minimize(int) (string, error) {
	return "", errors.New("native window control is unavailable")
}
func (unsupportedWindowController) Resume(string) error {
	return errors.New("native window control is unavailable")
}
