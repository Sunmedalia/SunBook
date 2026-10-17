package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/gdamore/tcell/v3"
	"sunbook/internal/config"
	"sunbook/internal/plugin"
	"sunbook/internal/terminal"
	"sunbook/internal/ui"
)

var version = "0.2.0"

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "sunbook:", err)
		os.Exit(1)
	}
}
func run() error {
	path := flag.String("config", config.Path(), "configuration file")
	checkPlugin := flag.String("check-plugin", "", "validate a local plugin handshake and health response")
	doctor := flag.Bool("doctor", false, "check optional external dependencies")
	showVersion := flag.Bool("version", false, "print version")
	flag.Usage = func() { fmt.Fprintln(os.Stderr, "Usage: sunbook [options] [file|directory]"); flag.PrintDefaults() }
	flag.Parse()
	if *showVersion {
		fmt.Println("SunBook", version)
		return nil
	}
	if flag.NArg() > 1 {
		return fmt.Errorf("expected at most one file or directory")
	}
	if *checkPlugin != "" {
		dir, err := filepath.Abs(*checkPlugin)
		if err != nil {
			return err
		}
		if err = plugin.Check(context.Background(), dir); err != nil {
			return err
		}
		fmt.Println("Plugin API v1 handshake and health check passed")
		return nil
	}
	cfg, err := config.Load(*path)
	if err != nil {
		return err
	}
	if *doctor {
		fmt.Print(config.Doctor(cfg))
		fmt.Println("Config:", *path)
		return nil
	}
	tty, err := tcell.NewDevTty()
	if err != nil {
		return err
	}
	colorTTY := terminal.NewColorTty(tty)
	screen, err := tcell.NewTerminfoScreenFromTty(colorTTY)
	if err != nil {
		_ = tty.Close()
		return err
	}
	if err = screen.Init(); err != nil {
		_ = tty.Close()
		return err
	}
	defer screen.Fini()
	colorTTY.RequestColors()
	app, err := ui.New(screen, cfg, flag.Arg(0))
	if err != nil {
		return err
	}
	return app.Run()
}
