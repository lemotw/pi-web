package main

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"strings"

	"pi-web/internal/render"
)

//go:embed all:web/dist
var distEmbed embed.FS

// distFS is the Vite build output rooted at "web/dist/", surfaced as if "/" were
// the dist directory. Run `npm --prefix web run build` before `go build` —
// otherwise the embed directive will fail at build time.
func distFS() fs.FS {
	sub, err := fs.Sub(distEmbed, "web/dist")
	if err != nil {
		panic(err)
	}
	return sub
}

const (
	indexEntry   = "src/index/index.js"
	sessionEntry = "src/session/session.js"
	liveEntry    = "src/live/live.js"
)

// frontendScript is one Vite-built JavaScript entrypoint ready to be served by Go.
type frontendScript struct {
	Entry string
	Path  string
	JS    string
}

func loadManifest(distFS fs.FS) (render.Manifest, error) {
	data, err := fs.ReadFile(distFS, ".vite/manifest.json")
	if err != nil {
		return nil, fmt.Errorf("read manifest: %w", err)
	}
	var manifest render.Manifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("parse manifest: %w", err)
	}
	return manifest, nil
}

func validateManifestEntry(manifest render.Manifest, entryName string) (render.ManifestEntry, error) {
	entry, ok := manifest[entryName]
	if !ok {
		return render.ManifestEntry{}, fmt.Errorf("manifest missing %s entry", entryName)
	}
	if entry.File == "" {
		return render.ManifestEntry{}, fmt.Errorf("manifest entry file is empty: %s", entryName)
	}
	if strings.HasPrefix(entry.File, "/") {
		return render.ManifestEntry{}, fmt.Errorf("manifest entry file is absolute: %s", entry.File)
	}
	if strings.Contains(entry.File, "..") {
		return render.ManifestEntry{}, fmt.Errorf("manifest entry file contains path traversal: %s", entry.File)
	}
	return entry, nil
}

func loadFrontendScript(distFS fs.FS, manifest render.Manifest, entryName string) (frontendScript, error) {
	entry, err := validateManifestEntry(manifest, entryName)
	if err != nil {
		return frontendScript{}, err
	}
	scriptPath, ok := manifest.ScriptPath(entryName)
	if !ok {
		return frontendScript{}, fmt.Errorf("manifest script path not found: %s", entryName)
	}
	content, err := fs.ReadFile(distFS, entry.File)
	if err != nil {
		return frontendScript{}, fmt.Errorf("read %s js: %w", entryName, err)
	}
	return frontendScript{Entry: entryName, Path: scriptPath, JS: string(content)}, nil
}

func loadFrontendScripts(distFS fs.FS, entryNames ...string) ([]frontendScript, error) {
	manifest, err := loadManifest(distFS)
	if err != nil {
		return nil, err
	}
	scripts := make([]frontendScript, 0, len(entryNames))
	for _, entryName := range entryNames {
		script, err := loadFrontendScript(distFS, manifest, entryName)
		if err != nil {
			return nil, err
		}
		scripts = append(scripts, script)
	}
	return scripts, nil
}

func serveIndexJS(js string, immutable bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
		if immutable {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			w.Header().Set("Cache-Control", "no-cache")
		}
		_, _ = w.Write([]byte(js))
	}
}
