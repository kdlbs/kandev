package lifecycle

import (
	"context"
	"errors"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	sprites "github.com/superfly/sprites-go"
)

func TestSpriteFileUploaderReadFileNormalizesMissingTarget(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/sprites/test/fs/read" {
			http.NotFound(w, r)
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(server.Close)

	client := sprites.New("token", sprites.WithBaseURL(server.URL), sprites.WithDisableControl())
	uploader := &spriteFileUploader{sprite: client.Sprite("test")}
	_, err := uploader.ReadFile(context.Background(), "/home/opencode/auth.json")
	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("ReadFile error = %v, want fs.ErrNotExist", err)
	}
}

func TestSpriteFileUploaderReadFileHonorsCancellation(t *testing.T) {
	started := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-r.Context().Done()
	}))
	t.Cleanup(server.Close)

	client := sprites.New("token", sprites.WithBaseURL(server.URL), sprites.WithDisableControl())
	uploader := &spriteFileUploader{sprite: client.Sprite("test")}
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		_, err := uploader.ReadFile(ctx, "/home/opencode/auth.json")
		result <- err
	}()

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("sprite read did not start")
	}
	cancel()

	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("ReadFile error = %v, want context.Canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("canceled sprite read did not return promptly")
	}
}
