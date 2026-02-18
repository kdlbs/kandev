package process

import (
	"context"
	"testing"
	"time"
)

func TestContainsDSRQuery(t *testing.T) {
	tests := []struct {
		name string
		data []byte
		want bool
	}{
		{"ESC[6n", []byte("\x1b[6n"), true},
		{"ESC[?6n", []byte("\x1b[?6n"), true},
		{"mixed content with DSR", []byte("text\x1b[6nmore"), true},
		{"no escape", []byte("hello world"), false},
		{"partial ESC[6", []byte("\x1b[6"), false},
		{"ESC[c is not DSR", []byte("\x1b[c"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := containsDSRQuery(tt.data); got != tt.want {
				t.Errorf("containsDSRQuery(%q) = %v, want %v", tt.data, got, tt.want)
			}
		})
	}
}

func TestContainsDA1Query(t *testing.T) {
	tests := []struct {
		name string
		data []byte
		want bool
	}{
		{"ESC[c (DA1 no param)", []byte("\x1b[c"), true},
		{"ESC[0c (DA1 explicit 0)", []byte("\x1b[0c"), true},
		{"mixed with DA1", []byte("text\x1b[cmore"), true},
		{"both DSR and DA1", []byte("\x1b[6n\x1b[c"), true},
		{"no escape", []byte("hello world"), false},
		{"ESC[1c is cursor forward, not DA1", []byte("\x1b[1c"), false},
		{"ESC[2c is cursor forward, not DA1", []byte("\x1b[2c"), false},
		{"ESC[5c is cursor forward, not DA1", []byte("\x1b[5c"), false},
		{"ESC[9c is cursor forward, not DA1", []byte("\x1b[9c"), false},
		{"partial ESC[", []byte("\x1b["), false},
		{"empty data", []byte{}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := containsDA1Query(tt.data); got != tt.want {
				t.Errorf("containsDA1Query(%q) = %v, want %v", tt.data, got, tt.want)
			}
		})
	}
}

func TestInteractiveRunner_ResizeByProcessID(t *testing.T) {
	log := newTestLogger(t)
	runner := NewInteractiveRunner(nil, log, 2*1024*1024)

	// Start a deferred process
	req := InteractiveStartRequest{
		SessionID: "resize-pid-test",
		Command:   []string{"cat"},
	}
	info, err := runner.Start(context.Background(), req)
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	// Process should not be running yet
	if runner.IsProcessRunning(info.ID) {
		t.Error("process should not be running before resize")
	}

	// Trigger start via ResizeByProcessID
	if err := runner.ResizeByProcessID(info.ID, 80, 24); err != nil {
		t.Fatalf("ResizeByProcessID() error = %v", err)
	}

	// Give process time to start
	time.Sleep(100 * time.Millisecond)

	if !runner.IsProcessRunning(info.ID) {
		t.Error("process should be running after ResizeByProcessID")
	}

	// Subsequent resize should succeed
	if err := runner.ResizeByProcessID(info.ID, 120, 40); err != nil {
		t.Errorf("second ResizeByProcessID() error = %v", err)
	}

	// Non-existent process should fail
	if err := runner.ResizeByProcessID("nonexistent", 80, 24); err == nil {
		t.Error("ResizeByProcessID() should fail for nonexistent process")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = runner.Stop(ctx, info.ID)
}
