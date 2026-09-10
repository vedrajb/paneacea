//go:build !windows

package terminal

import "errors"

func newConPTY(Options) (backend, error) {
	return nil, errors.New("ConPTY is only available on Windows")
}
