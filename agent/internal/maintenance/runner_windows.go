//go:build windows

package maintenance

import "errors"

func (b *Broker) start(_ string, _ func(error)) error {
	return errors.New("local maintenance is available only on a provisioned Linux ThinPi client")
}
