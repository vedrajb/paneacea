package main

import (
	"fmt"
	"github.com/paneacea/paneacea/frontend"
	"github.com/paneacea/paneacea/internal/applog"
	"github.com/paneacea/paneacea/internal/cli"
	"github.com/paneacea/paneacea/internal/desktop"
	"github.com/paneacea/paneacea/internal/ipc"
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"io/fs"
	"os"
)

func main() {
	if len(os.Args) > 1 {
		if err := cli.Run(os.Args[1:], os.Stdout); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	if logFile, err := applog.Setup("paneacea"); err == nil {
		defer logFile.Close()
	}
	app := desktop.New()
	// One window per install folder; launching again focuses it (it also owns the runtime and database).
	var singleInstance *options.SingleInstanceLock
	if id, err := ipc.InstanceID(); err == nil {
		singleInstance = &options.SingleInstanceLock{UniqueId: "paneacea-" + id, OnSecondInstanceLaunch: app.SecondInstance}
	}
	assets, err := fs.Sub(frontend.Assets, "dist")
	if err == nil {
		err = wails.Run(&options.App{Title: "Paneacea", Width: 1400, Height: 900, MinWidth: 760, MinHeight: 480, Frameless: true, BackgroundColour: options.NewRGB(30, 30, 30), AssetServer: &assetserver.Options{Assets: assets}, SingleInstanceLock: singleInstance, OnStartup: app.Startup, OnDomReady: app.DomReady, OnShutdown: app.Shutdown, Bind: []interface{}{app}})
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
