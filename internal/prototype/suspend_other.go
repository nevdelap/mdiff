//go:build plan9 || nacl || windows

package prototype

import "errors"

func suspendProcess() error {
	return errors.New("Suspend is unavailable on this platform")
}
