//go:build !windows

package maintenance

import (
	"errors"
	"os/exec"
)

func (b *Broker) start(user string, done func(error)) error {
	systemctl, err := exec.LookPath("systemctl")
	if err != nil {
		return errors.New("systemd maintenance broker is unavailable")
	}
	cmd := exec.Command(systemctl, "start", "--no-block", "thinpi-maintenance@"+user+".service")
	if err = cmd.Run(); err != nil {
		return errors.New("could not open the maintenance console")
	}
	done(nil)
	return nil
}
