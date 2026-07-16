package main

import (
	"context"
	"embed"
	"log/slog"
	"os"

	"github.com/BurntSushi/toml"
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	// Load initial config.
	cfg, err := loadAppConfig("config.toml")
	if err != nil {
		slog.Error("failed to load config", "err", err)
		os.Exit(1)
	}

	app := &App{
		ctx:    context.Background(),
		config: cfg,
	}

	err = wails.Run(&options.App{
		Title:  "DNSforVPN",
		Width:  960,
		Height: 680,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		OnStartup: func(ctx context.Context) {
			app.ctx = ctx
			slog.Info("DNSforVPN UI started")
		},
		OnShutdown: func(ctx context.Context) {
			app.Stop()
		},
		Bind: []interface{}{
			app,
		},
	})

	if err != nil {
		slog.Error("wails run failed", "err", err)
		os.Exit(1)
	}
}

func loadAppConfig(path string) (Config, error) {
	var cfg Config
	if _, err := toml.DecodeFile(path, &cfg); err != nil {
		return cfg, err
	}
	return cfg, nil
}
