package main

import (
	_ "embed"
	"net/http"
)

//go:embed live_templates/manifest.webmanifest
var manifestJSON string

//go:embed live_templates/sw.js
var swJS string

//go:embed live_templates/icon.svg
var iconSVG string

//go:embed live_templates/icon-maskable.svg
var iconMaskableSVG string

//go:embed live_templates/pi-logo.svg
var piLogoSVG string

//go:embed live_templates/done.mp3
var doneMP3 []byte

//go:embed live_templates/index.css
var indexCSS string

//go:embed live_templates/menu.css
var menuCSS string

// registerPWAHandlers serves the manifest, service worker, and icons.
// Routes are registered without auth: a manifest/icon leaks nothing
// sensitive, and the service worker must be reachable for installability
// even before the user authenticates.
func registerPWAHandlers(mux *http.ServeMux) {
	mux.HandleFunc("/manifest.webmanifest", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/manifest+json")
		w.Header().Set("Cache-Control", "no-cache")
		_, _ = w.Write([]byte(manifestJSON))
	})
	mux.HandleFunc("/sw.js", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
		// Allow the SW to control the whole origin.
		w.Header().Set("Service-Worker-Allowed", "/")
		w.Header().Set("Cache-Control", "no-cache")
		_, _ = w.Write([]byte(swJS))
	})
	mux.HandleFunc("/icon.svg", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/svg+xml")
		w.Header().Set("Cache-Control", "public, max-age=86400")
		_, _ = w.Write([]byte(iconSVG))
	})
	mux.HandleFunc("/icon-maskable.svg", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/svg+xml")
		w.Header().Set("Cache-Control", "public, max-age=86400")
		_, _ = w.Write([]byte(iconMaskableSVG))
	})
	mux.HandleFunc("/pi-logo.svg", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/svg+xml")
		w.Header().Set("Cache-Control", "public, max-age=86400")
		_, _ = w.Write([]byte(piLogoSVG))
	})
	mux.HandleFunc("/done.mp3", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "audio/mpeg")
		w.Header().Set("Cache-Control", "public, max-age=86400")
		_, _ = w.Write(doneMP3)
	})
	mux.HandleFunc("/index.css", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		_, _ = w.Write([]byte(indexCSS))
	})
	mux.HandleFunc("/menu.css", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		_, _ = w.Write([]byte(menuCSS))
	})
}
