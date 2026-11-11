package lifecycle

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/server/process"
	"github.com/stretchr/testify/require"
)

// Use the actual agentctl process runner: exited processes disappear, unlike
// a fake HTTP response that keeps an exit code available forever.
func TestRetainedProbeSurvivesRealProcessResultExpiry(t *testing.T) {
	for _, valid := range []bool{true, false} {
		t.Run(strconv.FormatBool(valid), func(t *testing.T) {
			root := t.TempDir()
			directory := filepath.Join(root, "secondary")
			require.NoError(t, os.MkdirAll(directory, 0o700))
			initGitRepoAt(t, directory)
			origin := "https://example.com/secondary.git"
			runGitForPreparerTest(t, directory, "remote", "add", "origin", origin)
			if !valid {
				runGitForPreparerTest(t, directory, "remote", "set-url", "origin", "https://example.com/wrong.git")
			}
			before := runGitForPreparerTest(t, directory, "show-ref") + runGitForPreparerTest(t, directory, "status", "--porcelain")
			runner := process.NewProcessRunner(nil, dialerTestLogger(t), 4096)
			t.Cleanup(func() {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				_ = runner.StopAllAndWait(ctx)
			})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/api/v1/processes/start":
					var req process.StartProcessRequest
					if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
						http.Error(w, "bad request", http.StatusBadRequest)
						return
					}
					info, err := runner.Start(context.Background(), req)
					if err != nil {
						http.Error(w, "start failed", http.StatusInternalServerError)
						return
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"process": info})
				case "/api/v1/processes/stop":
					var req process.StopProcessRequest
					_ = json.NewDecoder(r.Body).Decode(&req)
					_ = runner.Stop(r.Context(), req)
					_ = json.NewEncoder(w).Encode(map[string]bool{"success": true})
				default:
					// Reproduce the real elapsed time before the backend's first poll.
					time.Sleep(250 * time.Millisecond)
					info, ok := runner.Get(strings.TrimPrefix(r.URL.Path, "/api/v1/processes/"), r.URL.Query().Get("include_output") == "true")
					if !ok {
						http.Error(w, "process not found", http.StatusNotFound)
						return
					}
					_ = json.NewEncoder(w).Encode(info)
				}
			}))
			defer server.Close()
			u, err := url.Parse(server.URL)
			require.NoError(t, err)
			port, err := strconv.Atoi(u.Port())
			require.NoError(t, err)
			client := agentctl.NewClient(u.Hostname(), port, dialerTestLogger(t))
			defer client.Close()
			err = verifyRetainedLaunchWorkspace(context.Background(), "probe-session", &ExecutorInstance{Client: client, WorkspacePath: root}, []WorkspaceRepositoryMaterialization{{Destination: "secondary", RepositoryURL: origin, BaseBranch: "main"}})
			if valid {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, "workspace preserved")
			}
			require.Eventually(t, func() bool { return len(runner.List("probe-session")) == 0 }, 3*time.Second, 10*time.Millisecond)
			require.Equal(t, before, runGitForPreparerTest(t, directory, "show-ref")+runGitForPreparerTest(t, directory, "status", "--porcelain"))
		})
	}
}

func TestRetainedProbeRequiresItsOwnStdoutResult(t *testing.T) {
	marker := retainedProbeResultPrefix + "expected:"
	zero, one := 0, 1
	for _, tc := range []struct {
		name   string
		output []agentctl.ProcessOutputChunk
		want   *int
	}{
		{name: "split output", output: []agentctl.ProcessOutputChunk{{Stream: "stdout", Data: "\n" + marker[:10]}, {Stream: "stdout", Data: marker[10:] + "0\n"}}, want: &zero},
		{name: "failure", output: []agentctl.ProcessOutputChunk{{Stream: "stdout", Data: marker + "1\n"}}, want: &one},
		{name: "other nonce", output: []agentctl.ProcessOutputChunk{{Stream: "stdout", Data: retainedProbeResultPrefix + "other:0\n"}}},
		{name: "stderr", output: []agentctl.ProcessOutputChunk{{Stream: "stderr", Data: marker + "0\n"}}},
		{name: "malformed", output: []agentctl.ProcessOutputChunk{{Stream: "stdout", Data: marker + "0junk\n"}}},
		{name: "out of range", output: []agentctl.ProcessOutputChunk{{Stream: "stdout", Data: marker + "256\n"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, retainedProbeResult(tc.output, marker))
		})
	}
}
