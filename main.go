package main

import (
	"context"
	"embed"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	app := NewApp()

	err := wails.Run(&options.App{
		Title:         "DirectLink 加速器",
		Width:         480,
		Height:        720,
		MinWidth:      420,
		MinHeight:     640,
		DisableResize: false,
		Fullscreen:    false,
		Frameless:     false,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 24, G: 24, B: 28, A: 1},
		OnStartup: func(ctx context.Context) {
			app.Startup(ctx)
		},
		OnDomReady: func(ctx context.Context) {
			app.DomReady()
		},
		OnBeforeClose: func(ctx context.Context) bool {
			// Return true to prevent closing — minimize to tray instead
			app.HideToTray()
			return true
		},
		OnShutdown: func(ctx context.Context) {
			app.Shutdown()
		},
		Bind: []interface{}{
			app,
		},
	})

	if err != nil {
		println("Error:", err.Error())
	}
}
