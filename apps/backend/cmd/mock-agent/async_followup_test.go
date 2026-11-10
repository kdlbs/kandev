package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	acp "github.com/coder/acp-go-sdk"
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
		if !started || finished || completedFollowupTool(updates.getUpdates()) {
			t.Fatalf("before queue release: started=%v, finished=%v", started, finished)
		}
		if err := os.WriteFile(path, []byte("release"), 0o600); err != nil {
			t.Fatal(err)
		}
		time.Sleep(20 * time.Millisecond)
		synctest.Wait()
		for _, update := range updates.getUpdates() {
			finished = finished || getTextContent(update) == "Background follow-up finished."
		}
		if !finished || !completedFollowupTool(updates.getUpdates()) {
			t.Fatal("follow-up text and tool must finish after queue release")
		}
	})
}

func TestAsyncFollowupReleaseFailureRetainsTool(t *testing.T) {
	path := t.TempDir()
	synctest.Test(t, func(t *testing.T) {
		e, updates := newTestEmitter()
		handlePrompt(e, "/async-followup "+path, modelFast)
		time.Sleep(3 * time.Second)
		synctest.Wait()
		failed := false
		for _, update := range updates.getUpdates() {
			failed = failed || strings.HasPrefix(getTextContent(update), "Async follow-up release failed:")
		}
		if !failed || completedFollowupTool(updates.getUpdates()) {
			t.Fatal("release failure must stay visible and keep the tool active")
		}
	})
}

func TestAsyncFollowupAbsentReleaseMarkerTimesOut(t *testing.T) {
	path := filepath.Join(t.TempDir(), "never-released")
	synctest.Test(t, func(t *testing.T) {
		e, updates := newTestEmitter()
		defer func() {
			if err := os.WriteFile(path, []byte("release"), 0o600); err != nil {
				t.Error(err)
			}
		}()
		handlePrompt(e, "/async-followup "+path, modelFast)
		time.Sleep(2*time.Minute + 3*time.Second)
		synctest.Wait()
		failed := false
		for _, update := range updates.getUpdates() {
			text := getTextContent(update)
			failed = failed || (strings.HasPrefix(text, "Async follow-up release failed:") &&
				strings.Contains(text, context.DeadlineExceeded.Error()))
		}
		if !failed || completedFollowupTool(updates.getUpdates()) {
			t.Fatal("absent release marker must time out and keep the tool active")
		}
	})
}

func completedFollowupTool(updates []recordedUpdate) bool {
	for _, update := range updates {
		call := update.notification.Update.ToolCallUpdate
		if call != nil && call.ToolCallId == "async-followup-work" && call.Status != nil && *call.Status == acp.ToolCallStatusCompleted {
			return true
		}
	}
	return false
}
