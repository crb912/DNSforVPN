// dnsforvpn — DNS-over-HTTPS proxy with a built-in web UI.
//
// Usage:
//
//	dnsforvpn [--config PATH] [--no-browser]     run in foreground
//	dnsforvpn service ACTION [--config PATH]     manage the system service
//
// Service actions: install, uninstall, start, stop, restart, status.
// When run as a system service the browser is never opened and the
// configured paths are resolved relative to the config file.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"doh-dns-proxy/internal/browser"
	"doh-dns-proxy/internal/control"
	"doh-dns-proxy/internal/web"

	"github.com/kardianos/service"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "service" {
		serviceCommand(os.Args[2:])
		return
	}
	run(os.Args[1:])
}

// --- foreground / service runtime ---

type program struct {
	ctl        *control.Control
	web        *web.Server
	noBrowser  bool
	interactive bool
}

func (p *program) Start(svc service.Service) error {
	p.interactive = service.Interactive()
	go p.work()
	return nil
}

func (p *program) work() {
	cfg := p.ctl.GetConfig()

	w := web.New(p.ctl, cfg.Web)
	if err := w.Start(); err != nil {
		slog.Error("web UI failed to start", "err", err)
	} else {
		p.web = w
		slog.Info("web UI listening", "url", w.URL())
		if p.interactive && !p.noBrowser {
			if err := browser.Open(w.URL()); err != nil {
				slog.Warn("could not open browser", "err", err)
			}
		}
	}

	if err := p.ctl.Start(); err != nil {
		slog.Error("DNS server failed to start", "err", err)
	}
}

func (p *program) Stop(svc service.Service) error {
	if p.web != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = p.web.Shutdown(ctx)
	}
	p.ctl.Stop()
	return nil
}

func run(args []string) {
	fs := flag.NewFlagSet("dnsforvpn", flag.ExitOnError)
	cfgPath := fs.String("config", "configs/config.toml", "path to the config file")
	noBrowser := fs.Bool("no-browser", false, "do not open the web UI in a browser")
	_ = fs.Parse(args)

	ctl, err := control.New(*cfgPath)
	if err != nil {
		slog.Error("failed to load config", "path", *cfgPath, "err", err)
		os.Exit(1)
	}
	setupLogging(ctl.GetConfig().Logging.Level)

	absCfg, _ := filepath.Abs(*cfgPath)
	svc, err := service.New(&program{ctl: ctl, noBrowser: *noBrowser}, serviceConfig(absCfg))
	if err != nil {
		slog.Error("service init failed", "err", err)
		os.Exit(1)
	}

	if err := svc.Run(); err != nil {
		slog.Error("run failed", "err", err)
		os.Exit(1)
	}
}

// --- service management subcommand ---

func serviceCommand(args []string) {
	fs := flag.NewFlagSet("service", flag.ExitOnError)
	cfgPath := fs.String("config", "configs/config.toml", "path to the config file (recorded by install)")
	_ = fs.Parse(args)
	rest := fs.Args()
	if len(rest) != 1 {
		fmt.Fprintln(os.Stderr, "usage: dnsforvpn service install|uninstall|start|stop|restart|status [--config PATH]")
		os.Exit(2)
	}
	action := rest[0]

	absCfg, _ := filepath.Abs(*cfgPath)
	svc, err := service.New(&program{}, serviceConfig(absCfg))
	if err != nil {
		fatal(err)
	}

	switch action {
	case "install":
		err = svc.Install()
	case "uninstall":
		err = svc.Uninstall()
	case "start":
		err = svc.Start()
	case "stop":
		err = svc.Stop()
	case "restart":
		err = svc.Restart()
	case "status":
		st, serr := svc.Status()
		if serr != nil {
			fatal(serr)
		}
		switch st {
		case service.StatusRunning:
			fmt.Println("running")
		case service.StatusStopped:
			fmt.Println("stopped")
		default:
			fmt.Println("unknown")
		}
		return
	default:
		fmt.Fprintln(os.Stderr, "unknown action:", action)
		os.Exit(2)
	}
	if err != nil {
		fatal(err)
	}
	fmt.Println(action, "ok")
}

func serviceConfig(absCfgPath string) *service.Config {
	return &service.Config{
		Name:        "dnsforvpn",
		DisplayName: "DNSforVPN",
		Description: "DNS-over-HTTPS proxy with GFWList routing and a web UI",
		Arguments:   []string{"--config", absCfgPath, "--no-browser"},
		// Paths in the config file are resolved relative to its directory;
		// setting the working directory keeps any other relative use sane.
		WorkingDirectory: filepath.Dir(absCfgPath),
	}
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "error:", err)
	os.Exit(1)
}

func setupLogging(level string) {
	lvl := slog.LevelInfo
	switch level {
	case "debug":
		lvl = slog.LevelDebug
	case "warn":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: lvl,
	})))
}
