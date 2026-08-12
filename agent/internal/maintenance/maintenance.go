package maintenance

import (
	"context"
	"errors"
	"log/slog"
	"regexp"
	"sync"
	"time"
)

type Controller interface {
	RedeemMaintenance(context.Context, string) error
}

type Broker struct {
	controller Controller
	user       string
	log        *slog.Logger
	mu         sync.Mutex
	active     bool
}

var safeUser = regexp.MustCompile(`^[a-z_][a-z0-9_-]{0,31}$`)

func New(controller Controller, user string, log *slog.Logger) *Broker {
	return &Broker{controller: controller, user: user, log: log}
}

func (b *Broker) Open(ticket string) error {
	if ticket == "" || !safeUser.MatchString(b.user) || b.user == "root" || b.user == "thinpi" {
		return errors.New("local maintenance is not configured")
	}
	b.mu.Lock()
	if b.active {
		b.mu.Unlock()
		return errors.New("local maintenance is already active")
	}
	b.active = true
	b.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	err := b.controller.RedeemMaintenance(ctx, ticket)
	cancel()
	if err != nil {
		b.setInactive()
		return errors.New("maintenance authorisation was rejected")
	}
	if err = b.start(b.user, func(err error) {
		b.setInactive()
		if err != nil && b.log != nil {
			b.log.Warn("maintenance console exited", "result", "failed")
		}
	}); err != nil {
		b.setInactive()
		return err
	}
	if b.log != nil {
		b.log.Info("maintenance console accepted")
	}
	return nil
}

func (b *Broker) setInactive() {
	b.mu.Lock()
	b.active = false
	b.mu.Unlock()
}
