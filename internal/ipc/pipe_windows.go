package ipc

import (
	"context"
	"fmt"
	"github.com/Microsoft/go-winio"
	"golang.org/x/sys/windows"
	"net"
	"time"
)

func Address() (string, error) {
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return "", err
	}
	return `\\.\pipe\paneacea-go-` + user.User.Sid.String(), nil
}
func Listen() (net.Listener, error) {
	address, err := Address()
	if err != nil {
		return nil, err
	}
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return nil, err
	}
	return winio.ListenPipe(address, &winio.PipeConfig{SecurityDescriptor: fmt.Sprintf("D:P(A;;GA;;;%s)", user.User.Sid.String()), InputBufferSize: 65536, OutputBufferSize: 65536})
}
func Dial(ctx context.Context) (net.Conn, error) {
	address, err := Address()
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	return winio.DialPipeContext(ctx, address)
}
