package app

import (
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"gopkg.in/yaml.v3"
)

// DeployWatcher watches the apps directory for deployment state changes
// (creation, modification, or removal of .homelabctl.yaml files) and invokes
// a callback with the app name and current DeployedApp (nil when removed).
type DeployWatcher struct {
	appsDir  string
	watcher  *fsnotify.Watcher
	onChange func(appName string, info *DeployedApp)
	done     chan struct{}

	mu     sync.Mutex
	timers map[string]*time.Timer
}

// NewDeployWatcher creates a watcher on appsDir. The onChange callback receives
// the app name and the current *DeployedApp (or nil if the app was removed).
func NewDeployWatcher(appsDir string, onChange func(appName string, info *DeployedApp)) (*DeployWatcher, error) {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	dw := &DeployWatcher{
		appsDir:  appsDir,
		watcher:  w,
		onChange: onChange,
		done:     make(chan struct{}),
		timers:   make(map[string]*time.Timer),
	}

	// Watch the top-level apps directory for new/removed app dirs.
	if err := w.Add(appsDir); err != nil {
		w.Close()
		return nil, err
	}

	// Watch existing app directories for .homelabctl.yaml changes.
	entries, err := os.ReadDir(appsDir)
	if err == nil {
		for _, e := range entries {
			if e.IsDir() && !isHiddenDir(e.Name()) {
				_ = w.Add(filepath.Join(appsDir, e.Name()))
			}
		}
	}

	return dw, nil
}

// Start runs the event loop in a goroutine. Call Stop to shut it down.
func (dw *DeployWatcher) Start() {
	go dw.loop()
}

// Stop shuts down the watcher and cancels any pending debounce timers.
func (dw *DeployWatcher) Stop() {
	dw.watcher.Close()
	<-dw.done

	dw.mu.Lock()
	for name, t := range dw.timers {
		t.Stop()
		delete(dw.timers, name)
	}
	dw.mu.Unlock()
}

const debounceDelay = 500 * time.Millisecond

func (dw *DeployWatcher) loop() {
	defer close(dw.done)

	debounce := func(appName string) {
		dw.mu.Lock()
		defer dw.mu.Unlock()
		if t, ok := dw.timers[appName]; ok {
			t.Reset(debounceDelay)
			return
		}
		dw.timers[appName] = time.AfterFunc(debounceDelay, func() {
			dw.mu.Lock()
			delete(dw.timers, appName)
			dw.mu.Unlock()
			dw.reconcileApp(appName)
		})
	}

	for {
		select {
		case ev, ok := <-dw.watcher.Events:
			if !ok {
				return
			}
			appName, relevant := dw.classify(ev)
			if !relevant {
				continue
			}

			// If a new app directory appeared, start watching it.
			if ev.Op&fsnotify.Create != 0 {
				info, err := os.Stat(ev.Name)
				if err == nil && info.IsDir() && filepath.Dir(ev.Name) == dw.appsDir {
					_ = dw.watcher.Add(ev.Name)
				}
			}

			debounce(appName)

		case err, ok := <-dw.watcher.Errors:
			if !ok {
				return
			}
			slog.Warn("DeployWatcher error", "error", err)
		}
	}
}

// classify returns the app name and whether the event is relevant.
func (dw *DeployWatcher) classify(ev fsnotify.Event) (string, bool) {
	rel, err := filepath.Rel(dw.appsDir, ev.Name)
	if err != nil {
		return "", false
	}
	parts := splitPath(rel)
	if len(parts) == 0 {
		return "", false
	}

	appName := parts[0]
	if isHiddenDir(appName) {
		return "", false
	}

	// Events on the app dir itself (create/remove of the directory).
	if len(parts) == 1 {
		return appName, true
	}

	// Events on files inside the app dir — only care about .homelabctl.yaml.
	if len(parts) == 2 && parts[1] == ".homelabctl.yaml" {
		return appName, true
	}

	return "", false
}

func (dw *DeployWatcher) reconcileApp(appName string) {
	infoPath := filepath.Join(dw.appsDir, appName, ".homelabctl.yaml")
	data, err := os.ReadFile(infoPath)
	if err != nil {
		// File gone → app removed.
		dw.onChange(appName, nil)
		return
	}

	var info DeployedApp
	if err := yaml.Unmarshal(data, &info); err != nil {
		slog.Warn("DeployWatcher: failed to parse .homelabctl.yaml", "app", appName, "error", err)
		return
	}
	dw.onChange(appName, &info)
}

// splitPath splits a filepath into its components (filepath.SplitList is for
// PATH-style lists; this is a simple slash split).
func splitPath(p string) []string {
	var parts []string
	for p != "" && p != "." {
		dir, file := filepath.Split(p)
		if file != "" {
			parts = append([]string{file}, parts...)
		}
		p = filepath.Clean(dir)
		if p == "." || p == "/" {
			break
		}
	}
	return parts
}

func isHiddenDir(name string) bool {
	return name != "" && name[0] == '.'
}
