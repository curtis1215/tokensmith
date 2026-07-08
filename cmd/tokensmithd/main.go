// Command tokensmithd is the background token-harvest daemon. It continuously
// tails the local Claude Code and Codex logs and accumulates token usage into
// the ledger the game consumes (online and offline).
package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"tokensmith/internal/daemon"
	"tokensmith/internal/ledger"
)

// version is injected at build time via -ldflags "-X main.version=...".
var version = "dev"

func main() {
	if len(os.Args) > 1 && (os.Args[1] == "--version" || os.Args[1] == "-v" || os.Args[1] == "version") {
		fmt.Println("tokensmithd", version)
		return
	}
	home, _ := os.UserHomeDir()
	claude := filepath.Join(home, ".claude", "projects")
	codex := filepath.Join(home, ".codex", "sessions")
	lp := ledger.DefaultPath()

	if err := os.MkdirAll(filepath.Dir(lp), 0o755); err != nil {
		log.Fatal(err)
	}
	release, err := daemon.AcquireLock(lp + ".lock")
	if err != nil {
		log.Fatal(err)
	}
	defer release()

	h := daemon.New(claude, codex, lp)
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	log.Printf("tokensmithd harvesting → %s", lp)
	_ = h.Step(time.Now().Unix())
	for {
		select {
		case <-ticker.C:
			if err := h.Step(time.Now().Unix()); err != nil {
				log.Printf("step error: %v", err)
			}
		case <-stop:
			log.Print("shutting down")
			return
		}
	}
}
