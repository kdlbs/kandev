package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"testing/synctest"
)

func TestExecuteScriptAwaitFileBlocksUntilRelease(t *testing.T) {
	path := filepath.Join(t.TempDir(), "release")
	synctest.Test(t, func(t *testing.T) {
		e, updates := newTestEmitter()
		resetState()
		done := make(chan struct{})
		go func() {
			defer close(done)
			executeScript(e, "", fmt.Sprintf("e2e:message(\"waiting\")\ne2e:await_file(%q)\ne2e:message(\"released\")", path))
		}()
		synctest.Wait()
		if got := updates.getUpdates(); len(got) != 1 || getTextContent(got[0]) != "waiting" {
			t.Fatalf("updates before release = %+v, want only waiting", got)
		}
		select {
		case <-done:
			t.Fatal("script finished before the release file existed")
		default:
		}
		if err := os.WriteFile(path, []byte("release"), 0o600); err != nil {
			t.Fatal(err)
		}
		<-done
		got := updates.getUpdates()
		if len(got) != 2 || getTextContent(got[1]) != "released" {
			t.Fatalf("updates after release = %+v, want released", got)
		}
	})
}

func TestExecuteScriptAwaitFileHonorsCancellation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "never-released")
	synctest.Test(t, func(t *testing.T) {
		e, updates := newTestEmitter()
		resetState()
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		e.ctx = ctx
		done := make(chan struct{})
		go func() {
			defer close(done)
			executeScript(e, "", fmt.Sprintf("e2e:await_file(%q)\ne2e:message(\"unexpected\")", path))
		}()
		synctest.Wait()
		cancel()
		<-done
		if got := updates.getUpdates(); len(got) != 0 {
			t.Fatalf("canceled wait emitted updates: %+v", got)
		}
	})
}

func TestWaitForScriptFileRejectsInvalidMarker(t *testing.T) {
	for _, path := range []string{"", t.TempDir()} {
		if err := waitForScriptFile(context.Background(), path); err == nil {
			t.Fatalf("waitForScriptFile(%q) accepted an invalid marker", path)
		}
	}
}
