# k8s-filewatcher

A Go library for watching files with proper support for Kubernetes ConfigMap and Secret volume updates.

## Why This Library?

Kubernetes mounts ConfigMaps and Secrets using atomic symlink swaps via the `..data` directory pattern. When a ConfigMap or Secret is updated, kubelet:

1. Creates a new timestamped directory (e.g., `..data_tmp`)
2. Atomically renames it to `..data`
3. Deletes the old directory

This causes standard file watchers to miss updates or break. This library handles these symlink changes correctly using both fsnotify events and periodic polling for reliability.

## Installation

```bash
go get github.com/hadrienk/k8s-filewatcher
```

## Usage

### Basic Example

```go
package main

import (
    "context"
    "log"
    
    "github.com/hadrienk/k8s-filewatcher"
)

func main() {
    watcher, err := filewatcher.New("/var/run/secrets/kubernetes.io/serviceaccount/ca.crt")
    if err != nil {
        log.Fatal(err)
    }

    ctx := context.Background()
    go watcher.Start(ctx)

    // Read current content
    caCert := watcher.Get()
    log.Printf("CA cert: %d bytes", len(caCert))
}
```

### With Callback

```go
watcher, err := filewatcher.New("/path/to/config",
    filewatcher.WithInterval(5*time.Second),
    filewatcher.WithOnChange(func(content []byte) {
        log.Printf("File changed: %d bytes", len(content))
        // Reload your configuration here
    }),
)
```

### With File Info

```go
content, info, err := watcher.GetFS()
if err != nil {
    log.Fatal(err)
}
log.Printf("File: %s, Size: %d, Modified: %v", 
    info.Name(), info.Size(), info.ModTime())
```

## How It Works

The watcher uses a dual approach for maximum reliability:

1. **fsnotify events**: Detects Write, Create, Chmod, and Remove events. When a Remove or Chmod event occurs (indicating a symlink swap), the watch is re-added to follow the new symlink target.

2. **Periodic polling**: Every 10 seconds by default (configurable), the file is checked for changes. This catches any missed events and provides eventual consistency.

Thread safety is ensured using `sync.RWMutex` with read locks for `Get()` operations and write locks for updates.

## API

### Types

```go
type Watcher struct { /* ... */ }
type Option func(*Watcher)
```

### Functions

```go
// New creates a new file watcher
func New(path string, opts ...Option) (*Watcher, error)

// Options
func WithInterval(d time.Duration) Option
func WithOnChange(fn func(content []byte)) Option
```

### Methods

```go
// Start begins watching (blocks until context cancelled)
func (w *Watcher) Start(ctx context.Context) error

// Get returns current file content (thread-safe)
func (w *Watcher) Get() []byte

// GetFS returns content and file info (thread-safe)
func (w *Watcher) GetFS() ([]byte, fs.FileInfo, error)
```

## Testing

```bash
go test -v
```

The test suite includes:
- Basic file watching
- Callback invocation
- Thread safety
- Kubernetes symlink pattern simulation
- Context cancellation

## License

MIT License - see LICENSE file for details
