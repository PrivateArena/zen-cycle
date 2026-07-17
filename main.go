package main

import (
	"fmt"
	"log"
	"net"
	"os"

	"gioui.org/app"
	"gioui.org/unit"
)

var singleInstanceListener net.Listener

func main() {
	// 1. Single Instance Check via TCP Port binding (cross-platform, zero dependency)
	listener, err := net.Listen("tcp", "127.0.0.1:23953")
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error: Another instance of Zen-Cycle is already running.")
		os.Exit(1)
	}
	singleInstanceListener = listener
	defer singleInstanceListener.Close()

	// 2. Load configuration
	cfg, err := LoadConfig()
	if err != nil {
		log.Printf("Warning: Failed to load config, starting with defaults: %v", err)
	}
	if cfg == nil {
		cfg = &Config{}
	}

	// 3. Start Gio App
	go func() {
		w := new(app.Window)
		w.Option(app.Title("Zen-Cycle Account Manager"))
		w.Option(app.Size(unit.Dp(800), unit.Dp(600)))
		w.Option(app.MinSize(unit.Dp(600), unit.Dp(450)))
		
		if err := runUI(w, cfg); err != nil {
			log.Fatal(err)
		}
		os.Exit(0)
	}()

	app.Main()
}
