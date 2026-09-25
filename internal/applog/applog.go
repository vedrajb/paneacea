// Package applog writes process logs to a Logs folder beside the executable.
package applog

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
)

const maxLogSize = 10 << 20

// Setup directs the standard logger to <exe dir>/Logs/<name>.log, creating the folder if needed.
func Setup(name string) (io.Closer, error) {
	executable, err := os.Executable()
	if err != nil {
		return nil, err
	}
	return setupIn(filepath.Join(filepath.Dir(executable), "Logs"), name)
}

func setupIn(dir, name string) (io.Closer, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	path := filepath.Join(dir, name+".log")
	if info, err := os.Stat(path); err == nil && info.Size() > maxLogSize {
		_ = os.Remove(path + ".1")
		_ = os.Rename(path, path+".1")
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, err
	}
	log.SetOutput(file)
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)
	log.SetPrefix(fmt.Sprintf("[%s %d] ", name, os.Getpid()))
	return file, nil
}
