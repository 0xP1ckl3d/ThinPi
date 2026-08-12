//go:build !windows

package localapi

import (
	"errors"
	"net"
	"os"
	"path/filepath"
)

// ListenLocal exposes the agent through a permission-restricted Unix socket.
func ListenLocal(path string) (net.Listener, error) {
	if path == "" {
		return nil, errors.New("socket path is empty")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return nil, err
	}
	if info, err := os.Lstat(path); err == nil {
		if info.Mode()&os.ModeSocket == 0 {
			return nil, errors.New("refusing to replace a non-socket path")
		}
		if err = os.Remove(path); err != nil {
			return nil, err
		}
	}
	l, err := net.Listen("unix", path)
	if err != nil {
		return nil, err
	}
	if err = os.Chmod(path, 0660); err != nil {
		_ = l.Close()
		return nil, err
	}
	return l, nil
}
