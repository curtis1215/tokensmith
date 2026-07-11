// Command tokensmithd is the background token-harvest daemon. It continuously
// reads local Claude Code, Codex, Grok CLI, and OpenCode usage and accumulates
// token totals into the ledger the game consumes (online and offline).
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
	"tokensmith/internal/dailyusage"
	"tokensmith/internal/ingest"
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
	env := map[string]string{
		"CODEX_HOME":    os.Getenv("CODEX_HOME"),
		"GROK_HOME":     os.Getenv("GROK_HOME"),
		"XDG_DATA_HOME": os.Getenv("XDG_DATA_HOME"),
	}
	claudeRoots := []string{filepath.Join(home, ".claude", "projects")}
	codexRoots := ingest.CodexSessionRoots(home, env)
	snapshots := []ingest.SnapshotSource{
		ingest.NewGrokSnapshotSource(ingest.GrokHome(home, env)),
		ingest.NewOpenCodeSnapshotSource(ingest.OpenCodeDatabasePath(home, env)),
	}
	lp := ledger.DefaultPath()

	if err := os.MkdirAll(filepath.Dir(lp), 0o755); err != nil {
		log.Fatal(err)
	}
	release, err := daemon.AcquireLock(lp + ".lock")
	if err != nil {
		log.Fatal(err)
	}
	defer release()

	dailyPath := dailyusage.DefaultPath()
	dailyStore := dailyusage.New(dailyPath)
	h := daemon.NewWithSourcesAndDaily(claudeRoots, codexRoots, snapshots,
		dailyusage.NewBuffer(dailyStore), lp)
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	log.Printf("tokensmithd harvesting → %s", lp)
	log.Printf("tokensmithd daily usage → %s", dailyPath)
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
