package filewatcher

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestNew(t *testing.T) {
	t.Run("creates watcher for existing file", func(t *testing.T) {
		tmpFile := createTempFile(t, "test content")
		defer os.Remove(tmpFile)

		w, err := New(tmpFile)
		if err != nil {
			t.Fatalf("New() failed: %v", err)
		}
		if w == nil {
			t.Fatal("New() returned nil watcher")
		}

		content := w.Get()
		if string(content) != "test content" {
			t.Errorf("Get() = %q, want %q", content, "test content")
		}
	})

	t.Run("fails for non-existent file", func(t *testing.T) {
		_, err := New("/non/existent/file")
		if err == nil {
			t.Error("New() should fail for non-existent file")
		}
	})

	t.Run("accepts options", func(t *testing.T) {
		tmpFile := createTempFile(t, "test")
		defer os.Remove(tmpFile)

		w, err := New(tmpFile,
			WithInterval(1*time.Second),
			WithOnChange(func([]byte) {}),
		)
		if err != nil {
			t.Fatalf("New() with options failed: %v", err)
		}
		if w.interval != 1*time.Second {
			t.Errorf("interval = %v, want %v", w.interval, 1*time.Second)
		}
		if w.callback == nil {
			t.Error("callback not set")
		}
	})
}

func TestWatcher_Get(t *testing.T) {
	t.Run("returns current content", func(t *testing.T) {
		tmpFile := createTempFile(t, "initial")
		defer os.Remove(tmpFile)

		w, _ := New(tmpFile)
		if got := string(w.Get()); got != "initial" {
			t.Errorf("Get() = %q, want %q", got, "initial")
		}
	})

	t.Run("is thread-safe", func(t *testing.T) {
		tmpFile := createTempFile(t, "concurrent")
		defer os.Remove(tmpFile)

		w, _ := New(tmpFile)

		var wg sync.WaitGroup
		for i := 0; i < 100; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_ = w.Get()
			}()
		}
		wg.Wait()
	})
}

func TestWatcher_GetFS(t *testing.T) {
	t.Run("returns content and file info", func(t *testing.T) {
		tmpFile := createTempFile(t, "test")
		defer os.Remove(tmpFile)

		w, _ := New(tmpFile)
		content, info, err := w.GetFS()
		if err != nil {
			t.Fatalf("GetFS() error: %v", err)
		}
		if string(content) != "test" {
			t.Errorf("content = %q, want %q", content, "test")
		}
		if info == nil {
			t.Error("info is nil")
		}
	})
}

func TestWatcher_Start(t *testing.T) {
	t.Run("detects file writes", func(t *testing.T) {
		tmpFile := createTempFile(t, "v1")
		defer os.Remove(tmpFile)

		var callCount atomic.Int32
		w, _ := New(tmpFile,
			WithInterval(100*time.Millisecond),
			WithOnChange(func(content []byte) {
				callCount.Add(1)
			}),
		)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		go w.Start(ctx)
		time.Sleep(200 * time.Millisecond)

		os.WriteFile(tmpFile, []byte("v2"), 0644)
		time.Sleep(300 * time.Millisecond)

		if got := string(w.Get()); got != "v2" {
			t.Errorf("Get() = %q, want %q", got, "v2")
		}
		if callCount.Load() == 0 {
			t.Error("callback was not invoked")
		}
	})

	t.Run("detects kubernetes symlink updates", func(t *testing.T) {
		tmpDir := t.TempDir()

		dir1 := filepath.Join(tmpDir, "..data_1")
		os.Mkdir(dir1, 0755)
		os.WriteFile(filepath.Join(dir1, "ca.crt"), []byte("cert v1"), 0644)

		dataLink := filepath.Join(tmpDir, "..data")
		os.Symlink(dir1, dataLink)

		certLink := filepath.Join(tmpDir, "ca.crt")
		os.Symlink(filepath.Join("..data", "ca.crt"), certLink)

		var callCount atomic.Int32
		w, _ := New(certLink,
			WithInterval(100*time.Millisecond),
			WithOnChange(func(content []byte) {
				callCount.Add(1)
			}),
		)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		go w.Start(ctx)
		time.Sleep(200 * time.Millisecond)

		dir2 := filepath.Join(tmpDir, "..data_2")
		os.Mkdir(dir2, 0755)
		os.WriteFile(filepath.Join(dir2, "ca.crt"), []byte("cert v2"), 0644)

		os.Remove(dataLink)
		os.Symlink(dir2, dataLink)

		time.Sleep(500 * time.Millisecond)

		if got := string(w.Get()); got != "cert v2" {
			t.Errorf("Get() = %q, want %q after symlink update", got, "cert v2")
		}
		if callCount.Load() == 0 {
			t.Error("callback was not invoked after symlink update")
		}
	})

	t.Run("stops when context cancelled", func(t *testing.T) {
		tmpFile := createTempFile(t, "test")
		defer os.Remove(tmpFile)

		w, _ := New(tmpFile)
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()

		err := w.Start(ctx)
		if err != nil && err != context.DeadlineExceeded {
			t.Errorf("Start() unexpected error: %v", err)
		}
	})
}

func createTempFile(t *testing.T, content string) string {
	t.Helper()
	f, err := os.CreateTemp("", "filewatcher-test-*")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(content); err != nil {
		t.Fatal(err)
	}
	f.Close()
	return f.Name()
}
