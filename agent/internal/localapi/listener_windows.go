//go:build windows

package localapi

import (
	"errors"
	"fmt"
	"net"
	"os/user"
	"strings"

	"github.com/Microsoft/go-winio"
)

// ListenLocal exposes the agent through a named pipe accessible only to the
// current user, LocalSystem, and local administrators.
func ListenLocal(name string) (net.Listener, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, errors.New("named pipe is empty")
	}
	if !strings.HasPrefix(strings.ToLower(name), `\\.\pipe\`) {
		if strings.ContainsAny(name, `/\`) {
			return nil, errors.New("named pipe must be a simple name or a \\\\.\\pipe\\ path")
		}
		name = `\\.\pipe\` + name
	}
	current, err := user.Current()
	if err != nil {
		return nil, fmt.Errorf("identify current user: %w", err)
	}
	sid, err := winio.LookupSidByName(current.Username)
	if err != nil {
		return nil, fmt.Errorf("resolve current user SID: %w", err)
	}
	security := "D:P(A;;GA;;;" + sid + ")(A;;GA;;;SY)(A;;GA;;;BA)"
	return winio.ListenPipe(name, &winio.PipeConfig{
		SecurityDescriptor: security,
		InputBufferSize:    64 * 1024,
		OutputBufferSize:   64 * 1024,
	})
}
