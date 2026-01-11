// Package filewatcher provides a generic file watcher that handles Kubernetes
// ConfigMap and Secret volume symlink updates correctly.
//
// Kubernetes mounts ConfigMaps and Secrets using atomic symlink swaps via the
// ..data directory pattern. This watcher detects those changes using both
// fsnotify events and periodic polling for reliability.
package filewatcher

import (
	"bytes"
	"context"
	"fmt"
	"io/fs"
	"os"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

const defaultInterval = 10 * time.Second

type Watcher struct {
	sync.RWMutex

	path     string
	content  []byte
	modTime  time.Time
	callback func([]byte)

	watcher  *fsnotify.Watcher
	interval time.Duration
}

type Option func(*Watcher)

func WithInterval(d time.Duration) Option {
	return func(w *Watcher) {
		w.interval = d
	}
}

func WithOnChange(fn func(content []byte)) Option {
	return func(w *Watcher) {
		w.callback = fn
	}
}

func New(path string, opts ...Option) (*Watcher, error) {
	w := &Watcher{
		path:     path,
		interval: defaultInterval,
	}

	for _, opt := range opts {
		opt(w)
	}

	if err := w.reload(); err != nil {
		return nil, fmt.Errorf("failed to read initial file: %w", err)
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("failed to create fsnotify watcher: %w", err)
	}
	w.watcher = watcher

	return w, nil
}

func (w *Watcher) Get() []byte {
	w.RLock()
	defer w.RUnlock()
	return w.content
}

func (w *Watcher) GetFS() ([]byte, fs.FileInfo, error) {
	w.RLock()
	defer w.RUnlock()

	info := &fileInfo{
		name:    w.path,
		size:    int64(len(w.content)),
		modTime: w.modTime,
	}

	return w.content, info, nil
}

func (w *Watcher) Start(ctx context.Context) error {
	if err := w.addWatch(ctx); err != nil {
		return fmt.Errorf("failed to add watch: %w", err)
	}

	go w.watch()

	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			_ = w.watcher.Close()
			return nil
		case <-ticker.C:
			_ = w.reload()
		}
	}
}

func (w *Watcher) addWatch(ctx context.Context) error {
	timeout := 10 * time.Second
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		if err := w.watcher.Add(w.path); err == nil {
			return nil
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(1 * time.Second):
		}
	}

	return fmt.Errorf("failed to add watch after %v", timeout)
}

func (w *Watcher) watch() {
	for {
		select {
		case event, ok := <-w.watcher.Events:
			if !ok {
				return
			}
			w.handleEvent(event)
		case _, ok := <-w.watcher.Errors:
			if !ok {
				return
			}
		}
	}
}

func (w *Watcher) handleEvent(event fsnotify.Event) {
	switch {
	case event.Op&fsnotify.Write != 0:
	case event.Op&fsnotify.Create != 0:
	case event.Op&fsnotify.Chmod != 0, event.Op&fsnotify.Remove != 0:
		_ = w.watcher.Add(event.Name)
	default:
		return
	}

	_ = w.reload()
}

func (w *Watcher) reload() error {
	content, err := os.ReadFile(w.path)
	if err != nil {
		return err
	}

	info, err := os.Stat(w.path)
	if err != nil {
		return err
	}

	w.Lock()
	changed := !bytes.Equal(w.content, content)
	w.content = content
	w.modTime = info.ModTime()
	callback := w.callback
	w.Unlock()

	if changed && callback != nil {
		go callback(content)
	}

	return nil
}

type fileInfo struct {
	name    string
	size    int64
	modTime time.Time
}

func (fi *fileInfo) Name() string       { return fi.name }
func (fi *fileInfo) Size() int64        { return fi.size }
func (fi *fileInfo) Mode() fs.FileMode  { return 0644 }
func (fi *fileInfo) ModTime() time.Time { return fi.modTime }
func (fi *fileInfo) IsDir() bool        { return false }
func (fi *fileInfo) Sys() interface{}   { return nil }
