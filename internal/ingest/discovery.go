package ingest

import (
	"os"
	"path/filepath"
	"strings"
)

// CodexSessionRoots returns the ordered Codex session locations Tokensmith can
// discover without user registration. The Orca path is harmless on machines
// where Orca is absent: WalkDir will simply skip the missing directory.
func CodexSessionRoots(home string, env map[string]string) []string {
	var roots []string
	if codexHome := expandHome(env["CODEX_HOME"], home); codexHome != "" {
		roots = append(roots, filepath.Join(codexHome, "sessions"))
	}
	roots = append(roots,
		filepath.Join(home, ".codex", "sessions"),
		filepath.Join(home, "Library", "Application Support", "orca", "codex-runtime-home", "home", "sessions"),
	)
	return uniqueRoots(roots)
}

// GrokHome resolves standalone Grok CLI state without requiring registration.
func GrokHome(home string, env map[string]string) string {
	if custom := expandHome(env["GROK_HOME"], home); custom != "" {
		return filepath.Clean(custom)
	}
	return filepath.Join(home, ".grok")
}

// OpenCodeDatabasePath resolves OpenCode's XDG data database.
func OpenCodeDatabasePath(home string, env map[string]string) string {
	dataHome := expandHome(env["XDG_DATA_HOME"], home)
	if dataHome == "" {
		dataHome = filepath.Join(home, ".local", "share")
	}
	return filepath.Join(dataHome, "opencode", "opencode.db")
}

// NewDefaultSnapshotSources builds the zero-configuration mutable sources used
// by both tokensmithd and the standalone TUI fallback.
func NewDefaultSnapshotSources() []SnapshotSource {
	home, _ := os.UserHomeDir()
	env := envMap()
	return []SnapshotSource{
		NewGrokSnapshotSource(GrokHome(home, env)),
		NewOpenCodeSnapshotSource(OpenCodeDatabasePath(home, env)),
	}
}

func expandHome(path, home string) string {
	path = strings.TrimSpace(path)
	if path == "~" {
		return home
	}
	if strings.HasPrefix(path, "~/") {
		return filepath.Join(home, strings.TrimPrefix(path, "~/"))
	}
	return path
}

func envMap() map[string]string {
	env := make(map[string]string)
	for _, entry := range os.Environ() {
		key, value, ok := strings.Cut(entry, "=")
		if ok {
			env[key] = value
		}
	}
	return env
}
