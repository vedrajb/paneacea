package ipc

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Microsoft/go-winio"
	"golang.org/x/sys/windows"
)

func Address() (string, error) {
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return "", err
	}
	path, err := os.Executable()
	if err != nil {
		return "", err
	}
	directory, err := filepath.Abs(filepath.Dir(path))
	if err != nil {
		return "", err
	}
	return `\\.\pipe\paneacea-go-` + user.User.Sid.String() + "-" + pipeSuffix(directory), nil
}

func pipeSuffix(directory string) string {
	directory = strings.ToLower(filepath.Clean(directory))
	hash := sha256.Sum256([]byte(directory))
	return hex.EncodeToString(hash[:8])
}
func dialAddress() (string, error) {
	if address := os.Getenv("PANEACEA_PIPE"); address != "" {
		return address, nil
	}
	return Address()
}
func localDialAddress() (string, error) {
	return Address()
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
	address, err := dialAddress()
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	return winio.DialPipeContext(ctx, address)
}
func DialLocal(ctx context.Context) (net.Conn, error) {
	address, err := localDialAddress()
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	return winio.DialPipeContext(ctx, address)
}
