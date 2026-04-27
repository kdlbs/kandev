package configloader

import (
	"context"
	"log"
	"path/filepath"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

// DefaultDebounceDuration is the default debounce interval for filesystem events.
const DefaultDebounceDuration = 100 * time.Millisecond

// Watcher watches workspace directories for changes and triggers reloads.
type Watcher struct {
	loader   *ConfigLoader
	watcher  *fsnotify.Watcher
	debounce time.Duration
	onReload func(workspace string)
}

// NewWatcher creates a file system watcher backed by the given config loader.
func NewWatcher(loader *ConfigLoader) (*Watcher, error) {
	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	return &Watcher{
		loader:   loader,
		watcher:  fsw,
		debounce: DefaultDebounceDuration,
	}, nil
}

// SetDebounceDuration overrides the debounce interval (useful for tests).
func (w *Watcher) SetDebounceDuration(d time.Duration) {
	w.debounce = d
}

// SetOnReload registers a callback invoked after a workspace is reloaded.
// The callback receives the workspace name that was reloaded.
func (w *Watcher) SetOnReload(fn func(workspace string)) {
	w.onReload = fn
}

// Start begins watching all workspace directories. It blocks until ctx is cancelled.
func (w *Watcher) Start(ctx context.Context) error {
	if err := w.addWatchPaths(); err != nil {
		return err
	}
	w.eventLoop(ctx)
	return nil
}

// addWatchPaths registers the workspaces directory and each workspace's subdirectories.
func (w *Watcher) addWatchPaths() error {
	wsDir := filepath.Join(w.loader.BasePath(), "workspaces")
	if err := w.watcher.Add(wsDir); err != nil {
		return err
	}
	workspaces := w.loader.GetWorkspaces()
	for _, ws := range workspaces {
		w.addWorkspaceSubdirs(ws.DirPath)
	}
	return nil
}

// addWorkspaceSubdirs watches the workspace root and its immediate subdirectories.
func (w *Watcher) addWorkspaceSubdirs(wsPath string) {
	_ = w.watcher.Add(wsPath)
	for _, sub := range []string{"agents", "skills", "projects", "routines"} {
		_ = w.watcher.Add(filepath.Join(wsPath, sub))
	}
}

// eventLoop processes fsnotify events with debouncing.
func (w *Watcher) eventLoop(ctx context.Context) {
	var mu sync.Mutex
	pending := make(map[string]struct{})
	var timer *time.Timer

	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-w.watcher.Events:
			if !ok {
				return
			}
			wsName := w.loader.WorkspaceNameFromPath(event.Name)
			if wsName == "" {
				continue
			}
			mu.Lock()
			pending[wsName] = struct{}{}
			if timer != nil {
				timer.Stop()
			}
			timer = time.AfterFunc(w.debounce, func() {
				mu.Lock()
				names := make([]string, 0, len(pending))
				for n := range pending {
					names = append(names, n)
				}
				pending = make(map[string]struct{})
				mu.Unlock()
				for _, n := range names {
					if err := w.loader.Reload(n); err != nil {
						log.Printf("configloader: reload %s: %v", n, err)
						continue
					}
					if w.onReload != nil {
						w.onReload(n)
					}
				}
			})
			mu.Unlock()
		case err, ok := <-w.watcher.Errors:
			if !ok {
				return
			}
			log.Printf("configloader: watcher error: %v", err)
		}
	}
}

// Stop closes the underlying fsnotify watcher.
func (w *Watcher) Stop() error {
	return w.watcher.Close()
}
