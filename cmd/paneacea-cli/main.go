package main

import (
	"fmt"
	"github.com/paneacea/paneacea/internal/cli"
	"os"
)

func main() {
	if err := cli.Run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
