//go:build windows

package launch

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
)

type PlatformRunner struct{}

func (PlatformRunner) Run(ctx context.Context, c Command) error {
	if c.Path == "thinpi-mock" {
		return runMock(ctx, c)
	}
	if c.Password != "" {
		tool, err := exec.LookPath("vncpasswd.exe")
		if err != nil {
			return errors.New("vncpasswd is unavailable")
		}
		filter := exec.CommandContext(ctx, tool, "-f")
		filter.Stdin = strings.NewReader(c.Password + "\n")
		var encoded bytes.Buffer
		filter.Stdout = &encoded
		if err = filter.Run(); err != nil {
			return errors.New("could not prepare VNC credential")
		}
		file, err := os.CreateTemp("", "thinpi-vnc-password-*")
		if err != nil {
			return err
		}
		passwordFile := file.Name()
		defer os.Remove(passwordFile)
		if _, err = file.Write(encoded.Bytes()); err != nil {
			file.Close()
			return err
		}
		if err = file.Close(); err != nil {
			return err
		}
		for i := range c.Args {
			c.Args[i] = strings.ReplaceAll(c.Args[i], "{password_file}", passwordFile)
		}
		c.Password = ""
	}
	var materialFiles []string
	defer func() {
		for _, path := range materialFiles {
			_ = os.Remove(path)
		}
	}()
	for _, material := range c.Files {
		if material.Placeholder == "" || !strings.Contains(strings.Join(c.Args, "\x00"), material.Placeholder) {
			return errors.New("invalid protected-file placeholder")
		}
		file, err := os.CreateTemp("", "thinpi-session-material-*")
		if err != nil {
			return err
		}
		path := file.Name()
		materialFiles = append(materialFiles, path)
		if _, err = file.WriteString(material.Content); err != nil {
			file.Close()
			return err
		}
		if err = file.Close(); err != nil {
			return err
		}
		for i := range c.Args {
			c.Args[i] = strings.ReplaceAll(c.Args[i], material.Placeholder, path)
		}
	}
	return (ExecRunner{}).Run(ctx, c)
}
