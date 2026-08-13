//go:build !windows

package maintenance

import (
	"errors"
	"os"
	"os/exec"
	osuser "os/user"
	"strconv"
	"strings"
	"time"
)

func (b *Broker) start(user, terminalTheme string, done func(error)) error {
	systemctl, err := exec.LookPath("systemctl")
	if err != nil {
		return errors.New("systemd maintenance broker is unavailable")
	}
	unit := "thinpi-maintenance@" + user + ".service"
	if err = os.WriteFile("/run/thinpi/maintenance-theme", []byte(terminalTheme+"\n"), 0644); err != nil {
		return errors.New("could not configure the maintenance console")
	}
	xauthority, err := os.ReadFile("/home/thinpi/.Xauthority")
	if err != nil {
		return errors.New("kiosk display authorisation is unavailable")
	}
	account, err := osuser.Lookup(user)
	if err != nil {
		return errors.New("maintenance account is unavailable")
	}
	uid, uidErr := strconv.Atoi(account.Uid)
	gid, gidErr := strconv.Atoi(account.Gid)
	if uidErr != nil || gidErr != nil || os.WriteFile("/run/thinpi/maintenance.Xauthority", xauthority, 0600) != nil || os.Chown("/run/thinpi/maintenance.Xauthority", uid, gid) != nil {
		return errors.New("could not authorise the maintenance display")
	}
	cmd := exec.Command(systemctl, "start", "--no-block", unit)
	if err = cmd.Run(); err != nil {
		return errors.New("could not open the maintenance console")
	}

	// A queued systemd job is not proof that xterm reached the kiosk display.
	// Give the unit enough time to fail, then only report success while its
	// interactive shell is genuinely running.
	time.Sleep(500 * time.Millisecond)
	state, err := systemdUnitState(systemctl, unit)
	if err != nil || (state != "activating" && state != "active") {
		return errors.New("could not open the maintenance console")
	}
	go func() {
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()
		for range ticker.C {
			state, stateErr := systemdUnitState(systemctl, unit)
			if stateErr != nil {
				done(errors.New("maintenance console state could not be read"))
				return
			}
			if state != "activating" && state != "active" {
				done(nil)
				return
			}
		}
	}()
	return nil
}

func systemdUnitState(systemctl, unit string) (string, error) {
	out, err := exec.Command(systemctl, "show", "--property=ActiveState", "--value", unit).Output()
	return strings.TrimSpace(string(out)), err
}
