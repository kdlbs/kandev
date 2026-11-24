package providers

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

type notificationInvocation struct {
	Timeout  string
	Icon     string
	Operands []string
	Help     bool
}

// The executable parses options independently, including options after operands,
// as the installed notify-send parser does. It never contacts a desktop bus.
func init() {
	if os.Getenv("KANDEV_TEST_NOTIFY_HELPER") != "1" {
		return
	}
	mode := os.Getenv("KANDEV_TEST_NOTIFY_MODE")
	path := os.Getenv("KANDEV_TEST_NOTIFY_RECORD")
	if mode == "block" {
		if err := os.WriteFile(path, []byte("started"), 0600); err != nil {
			os.Exit(3)
		}
		for {
			time.Sleep(time.Hour)
		}
	}
	if mode == "fail" {
		os.Exit(17)
	}
	result := notificationInvocation{}
	options := true
	args := os.Args[1:]
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if options && arg == "--" {
			options = false
			continue
		}
		if options && (arg == "-t" || arg == "-i") && i+1 < len(args) {
			i++
			if arg == "-t" {
				result.Timeout = args[i]
			} else {
				result.Icon = args[i]
			}
			continue
		}
		if options && arg == "--help" {
			result.Help = true
			break
		}
		if options && len(arg) > 0 && arg[0] == '-' {
			os.Exit(2)
		}
		result.Operands = append(result.Operands, arg)
	}
	data, err := json.Marshal(result)
	if err != nil || os.WriteFile(path, data, 0600) != nil {
		os.Exit(3)
	}
	os.Exit(0)
}

func notificationFixture(t *testing.T) (string, Message) {
	t.Helper()
	dir := t.TempDir()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(executable, filepath.Join(dir, "notify-send")); err != nil {
		t.Fatal(err)
	}
	record := filepath.Join(dir, "invocation.json")
	t.Setenv("PATH", dir)
	t.Setenv("KANDEV_TEST_NOTIFY_HELPER", "1")
	t.Setenv("KANDEV_TEST_NOTIFY_RECORD", record)
	t.Setenv("KANDEV_TEST_NOTIFY_MODE", "record")
	t.Setenv("KANDEV_E2E_MOCK", "false")
	return record, Message{Config: map[string]interface{}{"timeout_ms": 3210, "icon_path": "/test/icon.png"}}
}

// @covers AC-PLATFORM-NOTIFICATIONS-001.13 AC-PLATFORM-NOTIFICATIONS-001.14 AC-PLATFORM-NOTIFICATIONS-001.15
func TestSystemProviderLinuxNotification(t *testing.T) {
	if isWSL() {
		t.Skip("SystemProvider uses the Windows transport under WSL")
	}
	for _, tc := range []struct{ name, title, body string }{
		{"ordinary", "Inbox: decision", "Review this"},
		{"help body", "Inbox: decision", "--help"},
		{"unknown body", "Inbox: decision", "--experimental"},
		{"option title", "--help", "decision"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			record, msg := notificationFixture(t)
			msg.Title, msg.Body = tc.title, tc.body
			if err := NewSystemProvider().Send(context.Background(), msg); err != nil {
				t.Fatalf("send: %v", err)
			}
			data, err := os.ReadFile(record)
			if err != nil {
				t.Fatal(err)
			}
			var got notificationInvocation
			if err := json.Unmarshal(data, &got); err != nil {
				t.Fatal(err)
			}
			want := notificationInvocation{Timeout: "3210", Icon: "/test/icon.png", Operands: []string{tc.title, tc.body}}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("parsed notification = %+v, want %+v", got, want)
			}
		})
	}
	t.Run("command failure", func(t *testing.T) {
		_, msg := notificationFixture(t)
		t.Setenv("KANDEV_TEST_NOTIFY_MODE", "fail")
		err := NewSystemProvider().Send(context.Background(), msg)
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) || exitErr.ExitCode() != 17 {
			t.Fatalf("send error = %v, want exit status 17", err)
		}
	})
	t.Run("cancellation", func(t *testing.T) {
		record, msg := notificationFixture(t)
		t.Setenv("KANDEV_TEST_NOTIFY_MODE", "block")
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		done := make(chan error, 1)
		go func() { done <- NewSystemProvider().Send(ctx, msg) }()
		t.Cleanup(func() { cancel(); <-done })
		deadline := time.NewTimer(5 * time.Second)
		defer deadline.Stop()
		ticker := time.NewTicker(10 * time.Millisecond)
		defer ticker.Stop()
		for {
			if _, err := os.Stat(record); err == nil {
				break
			} else if !os.IsNotExist(err) {
				t.Fatal(err)
			}
			select {
			case <-deadline.C:
				t.Fatal("notification child did not start")
			case <-ticker.C:
			}
		}
		cancel()
		select {
		case err := <-done:
			if err == nil || ctx.Err() != context.Canceled {
				t.Errorf("send error = %v, context error = %v", err, ctx.Err())
			}
			// Cleanup owns the join even when an assertion exits early.
			done <- err
		case <-time.After(5 * time.Second):
			t.Fatal("cancelled notification child did not exit")
		}
	})
	t.Run("e2e suppression", func(t *testing.T) {
		record, msg := notificationFixture(t)
		t.Setenv("KANDEV_E2E_MOCK", "true")
		provider := NewSystemProvider()
		if provider.Available() {
			t.Error("provider available during mock E2E")
		}
		if err := provider.Send(context.Background(), msg); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(record); !os.IsNotExist(err) {
			t.Fatalf("notification process ran or record could not be checked: %v", err)
		}
	})
}
