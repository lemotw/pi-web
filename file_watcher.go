package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

// watchFiles dispatches to fsnotify when available, falling back to polling.
// fsnotify gives ~ms-latency reloads; the poller is the safety net for
// platforms / filesystems where inotify or kqueue isn't available
// (e.g. some NFS mounts).
func (s *server) watchFiles() {
	if err := s.watchFilesFsnotify(); err != nil {
		fmt.Fprintf(os.Stderr, "fsnotify unavailable, falling back to polling: %v\n", err)
		s.watchFilesPolling()
	}
}

func (s *server) watchFilesPolling() {
	ticker := time.NewTicker(1500 * time.Millisecond)
	defer ticker.Stop()

	for range ticker.C {
		s.scanForChanges()
	}
}

func (s *server) scanForChanges() {
	entries, err := os.ReadDir(s.sessionsDir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		subDir := filepath.Join(s.sessionsDir, e.Name())
		subs, err := os.ReadDir(subDir)
		if err != nil {
			continue
		}
		for _, f := range subs {
			if f.IsDir() || !strings.HasSuffix(f.Name(), ".jsonl") {
				continue
			}
			path := filepath.Join(subDir, f.Name())
			info, err := os.Stat(path)
			if err != nil {
				continue
			}
			s.recordModTime(f.Name(), info.ModTime())
		}
	}
}

// recordModTime updates the last-known modtime for a session file and
// broadcasts a reload if it advanced. Shared between the polling and
// fsnotify paths so file-mod accounting stays consistent.
func (s *server) recordModTime(sessID string, mod time.Time) {
	s.fileModMu.Lock()
	lastMod, known := s.fileMod[sessID]
	s.fileMod[sessID] = mod
	s.fileModMu.Unlock()
	if known && mod.After(lastMod) {
		s.broadcast(sessID, "reload")
	}
}

// watchFilesFsnotify uses kqueue/inotify to react to writes. Project subdirs
// are watched individually (kqueue isn't recursive) and new ones are added
// dynamically when they appear under sessionsDir.
//
// To avoid sending two reloads for one logical write (editors emit multiple
// Write events), reloads are debounced per file with a short timer.
func (s *server) watchFilesFsnotify() error {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}

	if err := w.Add(s.sessionsDir); err != nil {
		_ = w.Close()
		return err
	}

	// Watch each existing project dir so we see writes to its jsonl files.
	if entries, err := os.ReadDir(s.sessionsDir); err == nil {
		for _, e := range entries {
			if e.IsDir() {
				_ = w.Add(filepath.Join(s.sessionsDir, e.Name()))
			}
		}
	}

	// Initial scan so the first real event isn't suppressed by an unknown
	// baseline modtime.
	s.scanForChanges()

	debouncers := newDebouncer(50 * time.Millisecond)
	go debouncers.run(s)
	go func() {
		defer w.Close()
		defer debouncers.stop()
		for {
			select {
			case ev, ok := <-w.Events:
				if !ok {
					return
				}
				s.handleFsEvent(w, ev, debouncers)
			case err, ok := <-w.Errors:
				if !ok {
					return
				}
				fmt.Fprintf(os.Stderr, "fsnotify error: %v\n", err)
			}
		}
	}()

	return nil
}

func (s *server) handleFsEvent(w *fsnotify.Watcher, ev fsnotify.Event, deb *debouncer) {
	// New project subdir → start watching it.
	if ev.Op&fsnotify.Create != 0 {
		if info, err := os.Stat(ev.Name); err == nil && info.IsDir() {
			_ = w.Add(ev.Name)
			return
		}
	}
	if !strings.HasSuffix(ev.Name, ".jsonl") {
		return
	}
	if ev.Op&(fsnotify.Write|fsnotify.Create) == 0 {
		return
	}
	// New session file → notify index-page subscribers so the list refreshes
	// without a manual reload. Only fires on Create events post-startup, since
	// the initial scan goes straight through scanForChanges/recordModTime.
	if ev.Op&fsnotify.Create != 0 {
		s.broadcast(globalSessID, "new-session")
	}
	deb.schedule(ev.Name)
}

// debouncer coalesces rapid bursts of events for the same file into a single
// reload. A timer per path is reset on each new event; once it fires, the
// reload is broadcast based on the file's current modtime.
type debouncer struct {
	delay  time.Duration
	mu     sync.Mutex
	timers map[string]*time.Timer
	stopCh chan struct{}
	wakeCh chan string
}

func newDebouncer(delay time.Duration) *debouncer {
	return &debouncer{
		delay:  delay,
		timers: make(map[string]*time.Timer),
		stopCh: make(chan struct{}),
		wakeCh: make(chan string, 16),
	}
}

func (d *debouncer) schedule(path string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if t, ok := d.timers[path]; ok {
		t.Reset(d.delay)
		return
	}
	d.timers[path] = time.AfterFunc(d.delay, func() {
		d.mu.Lock()
		delete(d.timers, path)
		d.mu.Unlock()
		select {
		case d.wakeCh <- path:
		case <-d.stopCh:
		}
	})
}

func (d *debouncer) run(s *server) {
	for {
		select {
		case path := <-d.wakeCh:
			info, err := os.Stat(path)
			if err != nil {
				continue
			}
			s.recordModTime(filepath.Base(path), info.ModTime())
		case <-d.stopCh:
			return
		}
	}
}

func (d *debouncer) stop() {
	close(d.stopCh)
	d.mu.Lock()
	for _, t := range d.timers {
		t.Stop()
	}
	d.timers = nil
	d.mu.Unlock()
}
