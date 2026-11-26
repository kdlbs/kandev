package plugins

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

const (
	installDeadlineTestWriteTimeout = 200 * time.Millisecond
	installDeadlineTestOriginDelay  = 500 * time.Millisecond
)

// Covers AC-PLUGINS-PLUGINS-001.22.
func TestInstallHandlerURLResponseSurvivesWriteTimeout(t *testing.T) {
	t.Run("delayed successful install", func(t *testing.T) {
		router, svc := newAdminTestRouter(t)
		archive := testPackage(t, "kandev-plugin-deadline", "1.0.0", false).Bytes()
		var originRequests atomic.Int32
		origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			if originRequests.Add(1) == 1 {
				w.Header().Set("Content-Length", strconv.Itoa(len(archive)))
				if _, err := w.Write(archive[:1]); err != nil {
					return
				}
				w.(http.Flusher).Flush()
				time.Sleep(installDeadlineTestOriginDelay)
				_, _ = w.Write(archive[1:])
				return
			}
			_, _ = w.Write(archive)
		}))
		t.Cleanup(origin.Close)

		server, deadlines := newInstallDeadlineTestServer(t, router)
		client := &http.Client{Timeout: 5 * time.Second}

		response, body := postInstallURL(t, client, server.URL, origin.URL)
		if response.StatusCode != http.StatusCreated {
			t.Fatalf("status = %d, want %d, body=%s", response.StatusCode, http.StatusCreated, body)
		}
		assertInstallResponseDeadline(t, deadlines, 0)
		var result InstallResponse
		if err := json.Unmarshal(body, &result); err != nil {
			t.Fatalf("decode install response: %v; body=%s", err, body)
		}
		if result.Plugin.ID != "kandev-plugin-deadline" || result.Plugin.Status != StatusActive {
			t.Fatalf("plugin = %+v, want active kandev-plugin-deadline", result.Plugin)
		}
		if _, err := svc.Get("kandev-plugin-deadline"); err != nil {
			t.Fatalf("plugin was not stored: %v", err)
		}

		response, body = postInstallURL(t, client, server.URL, origin.URL)
		if response.StatusCode != http.StatusConflict {
			t.Fatalf("repeat status = %d, want %d, body=%s", response.StatusCode, http.StatusConflict, body)
		}
		assertInstallResponseDeadline(t, deadlines, 1)
	})

	t.Run("delayed upstream error", func(t *testing.T) {
		router, svc := newAdminTestRouter(t)
		origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			time.Sleep(installDeadlineTestOriginDelay)
			w.WriteHeader(http.StatusBadGateway)
		}))
		t.Cleanup(origin.Close)

		server, deadlines := newInstallDeadlineTestServer(t, router)
		response, body := postInstallURL(t, &http.Client{Timeout: 5 * time.Second}, server.URL, origin.URL)
		if response.StatusCode != http.StatusInternalServerError {
			t.Fatalf("status = %d, want %d, body=%s", response.StatusCode, http.StatusInternalServerError, body)
		}
		assertInstallResponseDeadline(t, deadlines, 0)
		var result map[string]string
		if err := json.Unmarshal(body, &result); err != nil {
			t.Fatalf("decode install error: %v; body=%s", err, body)
		}
		if !strings.Contains(result["error"], "server responded 502") {
			t.Fatalf("error = %q, want upstream status in response", result["error"])
		}
		if plugins := svc.List(); len(plugins) != 0 {
			t.Fatalf("stored plugins = %+v, want none", plugins)
		}
	})
}

type installDeadlineResponseCapture struct {
	mu        sync.Mutex
	responses []*deadlineCaptureResponseWriter
}

func (c *installDeadlineResponseCapture) add(response *deadlineCaptureResponseWriter) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.responses = append(c.responses, response)
}

func (c *installDeadlineResponseCapture) at(index int) (time.Time, time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if index >= len(c.responses) {
		return time.Time{}, time.Time{}
	}
	return c.responses[index].deadlineAtResponseWrite()
}

type deadlineCaptureResponseWriter struct {
	http.ResponseWriter
	mu                sync.Mutex
	deadline          time.Time
	deadlineAtWrite   time.Time
	responseWrittenAt time.Time
}

func (w *deadlineCaptureResponseWriter) SetWriteDeadline(deadline time.Time) error {
	w.mu.Lock()
	w.deadline = deadline
	w.mu.Unlock()
	return http.NewResponseController(w.ResponseWriter).SetWriteDeadline(deadline)
}

func (w *deadlineCaptureResponseWriter) Write(body []byte) (int, error) {
	w.mu.Lock()
	w.deadlineAtWrite = w.deadline
	w.responseWrittenAt = time.Now()
	w.mu.Unlock()
	return w.ResponseWriter.Write(body)
}

func (w *deadlineCaptureResponseWriter) deadlineAtResponseWrite() (time.Time, time.Time) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.deadlineAtWrite, w.responseWrittenAt
}

func newInstallDeadlineTestServer(t *testing.T, handler http.Handler) (*httptest.Server, *installDeadlineResponseCapture) {
	t.Helper()
	deadlines := &installDeadlineResponseCapture{}
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capture := &deadlineCaptureResponseWriter{ResponseWriter: w}
		deadlines.add(capture)
		handler.ServeHTTP(capture, r)
	}))
	server.Config.WriteTimeout = installDeadlineTestWriteTimeout
	server.Start()
	t.Cleanup(server.Close)
	return server, deadlines
}

func assertInstallResponseDeadline(t *testing.T, deadlines *installDeadlineResponseCapture, index int) {
	t.Helper()
	deadline, writtenAt := deadlines.at(index)
	if deadline.IsZero() {
		t.Fatal("no write deadline was active at response write")
	}
	remaining := deadline.Sub(writtenAt)
	if remaining <= 0 || remaining > installDeadlineTestWriteTimeout {
		t.Fatalf("deadline at response write has %s remaining, want a fresh deadline within %s", remaining, installDeadlineTestWriteTimeout)
	}
}

func postInstallURL(t *testing.T, client *http.Client, endpoint, archiveURL string) (*http.Response, []byte) {
	t.Helper()
	requestBody := fmt.Sprintf(`{"url":%q}`, archiveURL)
	request, err := http.NewRequest(http.MethodPost, endpoint+"/api/plugins/install", bytes.NewBufferString(requestBody))
	if err != nil {
		t.Fatalf("create install request: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := client.Do(request)
	if err != nil {
		t.Fatalf("send install request: %v", err)
	}
	defer func() { _ = response.Body.Close() }()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read install response: %v", err)
	}
	return response, body
}
