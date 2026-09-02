package lsp

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestInstallCoordinatorSerializesSharedMutationKey(t *testing.T) {
	coordinator := newInstallCoordinator()
	entered := make(chan struct{})
	release := make(chan struct{})
	var active atomic.Int32
	var overlap atomic.Bool
	first := func() (string, error) {
		if active.Add(1) != 1 {
			overlap.Store(true)
		}
		close(entered)
		<-release
		active.Add(-1)
		return "first", nil
	}
	second := func() (string, error) {
		if active.Add(1) != 1 {
			overlap.Store(true)
		}
		active.Add(-1)
		return "second", nil
	}
	firstDone := make(chan error, 1)
	go func() {
		_, err := coordinator.run(context.Background(), "npm-prefix", first)
		firstDone <- err
	}()
	<-entered
	secondDone := make(chan error, 1)
	go func() {
		_, err := coordinator.run(context.Background(), "npm-prefix", second)
		secondDone <- err
	}()
	close(release)
	if err := <-firstDone; err != nil {
		t.Fatal(err)
	}
	if err := <-secondDone; err != nil {
		t.Fatal(err)
	}
	if overlap.Load() {
		t.Fatal("shared install mutations overlapped")
	}
}

func TestInstallCoordinatorCanceledWaiterDoesNotRun(t *testing.T) {
	coordinator := newInstallCoordinator()
	entered := make(chan struct{})
	release := make(chan struct{})
	go func() {
		_, _ = coordinator.run(context.Background(), "npm-prefix", func() (string, error) {
			close(entered)
			<-release
			return "first", nil
		})
	}()
	<-entered
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var ran atomic.Bool
	_, err := coordinator.run(ctx, "npm-prefix", func() (string, error) {
		ran.Store(true)
		return "second", nil
	})
	close(release)
	if !errors.Is(err, context.Canceled) || ran.Load() {
		t.Fatalf("error=%v ran=%v", err, ran.Load())
	}
	<-time.After(10 * time.Millisecond)
}

func TestInstallMutationKeysGroupSharedCaches(t *testing.T) {
	if installMutationKey("typescript") != installMutationKey("python") {
		t.Fatal("npm installers must share one mutation key")
	}
	if installMutationKey("go") == installMutationKey("rust") {
		t.Fatal("independent installer targets must not share a key")
	}
}
