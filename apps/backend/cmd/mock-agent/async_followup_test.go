package main

import (
	"os"
	"path/filepath"
	"testing"
	"testing/synctest"
	"time"
)

func TestAsyncFollowupWaitsForQueuedMessageRelease(t *testing.T) {
	path := filepath.Join(t.TempDir(), "queued.release")
	synctest.Test(t, func(t *testing.T) {
		e, updates := newTestEmitter()
		handlePrompt(e, "/async-followup "+path, modelFast)
		time.Sleep(45 * time.Second)
		synctest.Wait()
		started, finished := false, false
		for _, update := range updates.getUpdates() {
			started = started || getTextContent(update) == "Background follow-up started."
			finished = finished || getTextContent(update) == "Background follow-up finished."
		}
		if !started || finished {
			t.Fatalf("before queue release: started=%v, finished=%v", started, finished)
		}
		if err := os.WriteFile(path, []byte("release"), 0o600); err != nil {
			t.Fatal(err)
		}
		time.Sleep(20 * time.Millisecond)
		synctest.Wait()
		for _, update := range updates.getUpdates() {
			if getTextContent(update) == "Background follow-up finished." {
				return
			}
		}
		t.Fatal("follow-up did not finish after queue release")
	})
}
