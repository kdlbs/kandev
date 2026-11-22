package process

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"testing/synctest"
	"time"
)

func TestFilePreviewPoolExpiryAndReplay(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "model.stl"), []byte("mesh"), 0600); err != nil {
			t.Fatal(err)
		}
		pool := newFilePreviewPool()
		defer pool.close()
		p, err := prepareFilePreview(dir, "model.stl")
		if err != nil {
			t.Fatal(err)
		}
		token, err := pool.add(p)
		if err != nil {
			_ = p.Close()
			t.Fatal(err)
		}
		var out bytes.Buffer
		if err := pool.copyTo(t.Context(), token, &out); err != nil {
			t.Fatal(err)
		}
		if err := pool.copyTo(t.Context(), token, &out); err == nil {
			t.Fatal("receipt replay succeeded")
		}
		p, err = prepareFilePreview(dir, "model.stl")
		if err != nil {
			t.Fatal(err)
		}
		token, err = pool.add(p)
		if err != nil {
			_ = p.Close()
			t.Fatal(err)
		}
		time.Sleep(5 * time.Minute)
		synctest.Wait()
		if err := pool.copyTo(t.Context(), token, &out); err == nil {
			t.Fatal("expired receipt succeeded")
		}
		if p.file != nil {
			t.Fatal("expired file handle remains open")
		}
	})
}

func TestFilePreviewPoolCapacityAndClose(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "model.stl"), []byte("mesh"), 0600); err != nil {
		t.Fatal(err)
	}
	pool := newFilePreviewPool()
	defer pool.close()
	var tokens []string
	for i := 0; i < 5; i++ {
		p, err := prepareFilePreview(dir, "model.stl")
		if err != nil {
			t.Fatal(err)
		}
		token, err := pool.add(p)
		if i == 4 {
			_ = p.Close()
			if err == nil {
				t.Fatal("unbounded pending handles")
			}
			break
		}
		if err != nil {
			_ = p.Close()
			t.Fatal(err)
		}
		tokens = append(tokens, token)
	}
	pool.close()
	if err := pool.copyTo(t.Context(), tokens[0], &bytes.Buffer{}); err == nil {
		t.Fatal("closed pool read")
	}
}
