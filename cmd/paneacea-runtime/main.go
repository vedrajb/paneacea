package main

import (
	"context"
	"flag"
	"fmt"
	"github.com/paneacea/paneacea/internal/daemon"
	"github.com/paneacea/paneacea/internal/ipc"
	"github.com/paneacea/paneacea/internal/persistence"
	"os"
	"os/signal"
	"path/filepath"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run() error {
	data := flag.String("data", "", "SQLite state directory")
	flag.Parse()
	if *data == "" {
		*data = os.Getenv("PANEACEA_DATA_DIR")
	}
	if *data == "" {
		base, err := os.UserConfigDir()
		if err != nil {
			return err
		}
		*data = filepath.Join(base, "Paneacea", "go-runtime")
	}
	listener, err := ipc.Listen()
	if err != nil {
		return err
	}
	defer listener.Close()
	if err = os.MkdirAll(*data, 0700); err != nil {
		return err
	}
	store, err := persistence.Open(filepath.Join(*data, "paneacea.db"))
	if err != nil {
		return err
	}
	defer store.Close()
	runtime, err := daemon.New(store)
	if err != nil {
		return err
	}
	defer runtime.Close()
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	return daemon.Serve(ctx, listener, runtime)
}
