package process

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

func TestGitOperatorAuthenticationFailureAndRecovery(t *testing.T) {
	repoDir, cleanup := setupTestRepo(t)
	t.Cleanup(cleanup)
	remoteDir := strings.TrimSpace(runGit(t, repoDir, "remote", "get-url", "origin"))

	var unauthorized atomic.Int32
	var authenticated atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		username, password, ok := request.BasicAuth()
		if !ok || username != "x-access-token" || password != "good-token" {
			unauthorized.Add(1)
			response.Header().Set("WWW-Authenticate", `Basic realm="git-operator-test"`)
			response.WriteHeader(http.StatusUnauthorized)
			return
		}
		authenticated.Add(1)
		serveProcessGitHTTPBackend(response, request, filepath.Dir(remoteDir))
	}))
	t.Cleanup(server.Close)

	remoteURL := server.URL + "/" + filepath.Base(remoteDir)
	runGit(t, repoDir, "remote", "set-url", "origin", remoteURL)

	credential := "bad-token"
	baseEnv := filterTestGitEnv(os.Environ())
	baseEnv = append(baseEnv,
		"HOME="+t.TempDir(),
		"GIT_CONFIG_GLOBAL="+os.DevNull,
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_CONFIG_COUNT=2",
		"GIT_CONFIG_KEY_0=credential.helper",
		"GIT_CONFIG_VALUE_0=",
		"GIT_CONFIG_KEY_1=credential."+server.URL+".helper",
	)
	operator := NewGitOperator(repoDir, newTestLogger(t), nil)
	operator.setEnvironmentProvider(func() []string {
		env := append([]string(nil), baseEnv...)
		env = append(env, "GIT_CONFIG_VALUE_1="+gitTestCredentialHelper(credential))
		return env
	})

	first, err := operator.Pull(context.Background(), false)
	if err != nil {
		t.Fatalf("first Pull() returned error: %v", err)
	}
	if first == nil || first.Success {
		t.Fatalf("first Pull() = %+v, want required authentication failure", first)
	}
	if strings.Contains(first.Error+first.Output, "bad-token") {
		t.Fatalf("first Pull() leaked rejected credential: %+v", first)
	}
	if unauthorized.Load() == 0 {
		t.Fatal("HTTP fixture did not observe the rejected credential")
	}
	if !operator.tryLock("after-auth-failure") {
		t.Fatal("GitOperator operation lock was not released after required failure")
	}
	operator.unlock()

	credential = "good-token"
	second, err := operator.Pull(context.Background(), false)
	if err != nil {
		t.Fatalf("recovery Pull() returned error: %v", err)
	}
	if second == nil || !second.Success {
		t.Fatalf("recovery Pull() = %+v, want success", second)
	}
	if authenticated.Load() == 0 {
		t.Fatal("HTTP fixture did not observe the restored credential")
	}
}

func gitTestCredentialHelper(token string) string {
	return fmt.Sprintf("!f() { printf 'username=x-access-token\\npassword=%s\\n'; }; f", token)
}

func serveProcessGitHTTPBackend(response http.ResponseWriter, request *http.Request, root string) {
	cmd := exec.Command("git", "http-backend")
	cmd.Env = append(os.Environ(),
		"GIT_PROJECT_ROOT="+root,
		"GIT_HTTP_EXPORT_ALL=1",
		"PATH_INFO="+request.URL.Path,
		"QUERY_STRING="+request.URL.RawQuery,
		"REQUEST_METHOD="+request.Method,
		"REMOTE_ADDR=127.0.0.1",
		"CONTENT_TYPE="+request.Header.Get("Content-Type"),
		"CONTENT_LENGTH="+request.Header.Get("Content-Length"),
	)
	cmd.Stdin = request.Body
	output, err := cmd.Output()
	if err != nil {
		response.WriteHeader(http.StatusBadGateway)
		_, _ = fmt.Fprintf(response, "git http-backend failed: %v", err)
		return
	}
	outputString := string(output)
	headerEnd := strings.Index(outputString, "\r\n\r\n")
	delimiterLength := 4
	if headerEnd < 0 {
		headerEnd = strings.Index(outputString, "\n\n")
		delimiterLength = 2
	}
	if headerEnd < 0 {
		response.WriteHeader(http.StatusBadGateway)
		return
	}
	for _, line := range strings.Split(outputString[:headerEnd], "\n") {
		line = strings.TrimSuffix(line, "\r")
		key, value, ok := strings.Cut(line, ": ")
		if !ok {
			continue
		}
		if key == "Status" {
			var status int
			_, _ = fmt.Sscanf(value, "%d", &status)
			if status != 0 {
				response.WriteHeader(status)
			}
			continue
		}
		response.Header().Set(key, value)
	}
	_, _ = response.Write(output[headerEnd+delimiterLength:])
}
