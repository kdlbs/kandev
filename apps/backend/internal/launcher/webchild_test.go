package launcher

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestWebCommandUnix(t *testing.T) {
	cmd, args := webCommandFor("linux")
	if cmd != "pnpm" {
		t.Fatalf("command = %q, want pnpm", cmd)
	}
	want := []string{"-C", "apps", "--filter", "@kandev/web", "dev"}
	if len(args) != len(want) {
		t.Fatalf("args = %v, want %v", args, want)
	}
	for i := range want {
		if args[i] != want[i] {
			t.Fatalf("args = %v, want %v", args, want)
		}
	}
}

func TestWebCommandWindowsUsesCmdExeShim(t *testing.T) {
	cmd, args := webCommandFor("windows")
	if cmd != "cmd.exe" {
		t.Fatalf("command = %q, want cmd.exe", cmd)
	}
	want := []string{"/c", "pnpm", "-C", "apps", "--filter", "@kandev/web", "dev"}
	if len(args) != len(want) {
		t.Fatalf("args = %v, want %v", args, want)
	}
	for i := range want {
		if args[i] != want[i] {
			t.Fatalf("args = %v, want %v", args, want)
		}
	}
}

func TestBuildWebEnvSetsDevServerConfiguration(t *testing.T) {
	t.Setenv("VITE_KANDEV_API_PORT", "9999")
	t.Setenv("PORT", "8080")

	env, err := buildWebEnv(portConfig{
		BackendPort: 38429,
		WebPort:     37429,
		BackendURL:  "http://localhost:38429",
	}, true)
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(env, "\n")
	for _, expected := range []string{
		"KANDEV_API_BASE_URL=http://localhost:38429",
		"PORT=37429",
		"HOSTNAME=127.0.0.1",
		"KANDEV_DEBUG=true",
		"VITE_KANDEV_DEBUG=true",
	} {
		if !strings.Contains(joined, expected) {
			t.Fatalf("web environment missing %q", expected)
		}
	}
	if strings.Contains(joined, "VITE_KANDEV_API_PORT") {
		t.Fatalf("web environment leaked inherited VITE_KANDEV_API_PORT:\n%s", joined)
	}
	if strings.Contains(joined, "PORT=8080\n") || !strings.Contains(joined, "PORT=37429") {
		t.Fatalf("web environment did not override inherited PORT:\n%s", joined)
	}
}

func TestBuildWebEnvRequiresWebPort(t *testing.T) {
	if _, err := buildWebEnv(portConfig{BackendPort: 38429, BackendURL: "http://localhost:38429"}, false); err == nil {
		t.Fatal("buildWebEnv() = nil error without a web port")
	}
}

func TestWaitForURLReadyAcceptsAnyHTTPResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte("vite fallback"))
	}))
	t.Cleanup(srv.Close)

	err := waitForURLReady(context.Background(), srv.URL, fakeChild{}, 5*time.Second)
	if err != nil {
		t.Fatalf("waitForURLReady() = %v, want nil for any HTTP response", err)
	}
}

func TestWaitForURLReadyTimesOutWithURLNamed(t *testing.T) {
	err := waitForURLReady(context.Background(), "http://127.0.0.1:1", fakeChild{}, 300*time.Millisecond)
	if err == nil {
		t.Fatal("waitForURLReady() = nil, want timeout error")
	}
	if !strings.Contains(err.Error(), "timed out") || !strings.Contains(err.Error(), "http://127.0.0.1:1") {
		t.Fatalf("error = %v, want a timeout naming the URL", err)
	}
}

func TestWaitForURLReadyFailsFastWhenChildExited(t *testing.T) {
	err := waitForURLReady(
		context.Background(),
		"http://127.0.0.1:1",
		fakeChild{exited: true, code: 7},
		time.Minute,
	)
	if err == nil {
		t.Fatal("waitForURLReady() = nil, want exit error")
	}
	if !strings.Contains(err.Error(), "code 7") {
		t.Fatalf("error = %v, want the child exit code", err)
	}
}

func TestWaitForURLReadyHonorsContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := waitForURLReady(ctx, "http://127.0.0.1:1", fakeChild{}, time.Minute)
	if err == nil {
		t.Fatal("waitForURLReady() = nil, want cancellation error")
	}
}
