package lifecycle

import (
	"crypto/subtle"
	"fmt"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync/atomic"
	"time"

	"golang.org/x/crypto/ssh"
)

var sshManagedEnvKeys = []string{
	"KANDEV_API_URL", "KANDEV_API_KEY", "KANDEV_RUN_TOKEN", "KANDEV_AGENT_ID",
	"KANDEV_AGENT_NAME", "KANDEV_WORKSPACE_ID", "KANDEV_RUN_ID", "KANDEV_WAKE_REASON",
	"KANDEV_TASK_ID", "KANDEV_WAKE_COMMENT_ID", "KANDEV_RUNTIME_API_PREFIX",
}

const sshManagedHTTPScheme = "http"

// Managed agents call the local control plane through a run-authenticated SSH
// reverse listener. No publicly reachable backend URL or copied account login
// is required. The listener and active requests die with this SSH connection.
func prepareSSHManagedRuntime(client *ssh.Client, req *ExecutorCreateRequest, cli string) (*sshManagedRuntime, error) {
	if req.Env["KANDEV_RUN_TOKEN"] == "" {
		return nil, nil
	}
	target, err := url.Parse(req.Env["KANDEV_API_URL"])
	if err != nil || target == nil || target.Host == "" || (target.Scheme != sshManagedHTTPScheme && target.Scheme != "https") {
		return nil, fmt.Errorf("ssh managed runtime requires a valid API URL")
	}
	listener, err := client.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("ssh managed API forwarding (AllowTcpForwarding must permit remote forwarding): %w", err)
	}
	runtime := &sshManagedRuntime{cli: cli}
	runtime.token.Store(req.Env["KANDEV_RUN_TOKEN"])
	runtime.runEnv.Store(sshManagedRunEnv(req.Env))
	server := &http.Server{
		Handler:           sshManagedProxyWithToken(target, func() string { return runtime.token.Load().(string) }),
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	go func() { _ = server.Serve(listener) }()
	go func() { _ = client.Wait(); _ = server.Close() }()
	remote := *target
	remote.Scheme = sshManagedHTTPScheme
	_, port, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		_ = server.Close()
		return nil, err
	}
	remote.Host = net.JoinHostPort("127.0.0.1", port)
	env := make(map[string]string, len(req.Env))
	for key, value := range req.Env {
		env[key] = value
	}
	env["KANDEV_API_URL"] = remote.String()
	runtime.apiURL = remote.String()
	req.Env = env
	return runtime, nil
}

type sshManagedRuntime struct {
	token  atomic.Value
	runEnv atomic.Value
	apiURL string
	cli    string
}

func (r *sshManagedRuntime) envPreparer() func(map[string]string) error {
	if r == nil {
		return nil
	}
	return r.prepareEnv
}

func (r *sshManagedRuntime) prepareEnv(env map[string]string) error {
	token := env["KANDEV_RUN_TOKEN"]
	if token == "" {
		if env["KANDEV_RUN_ID"] != "" {
			return fmt.Errorf("SSH managed subprocess requires complete run credentials")
		}
		if current, ok := r.runEnv.Load().(map[string]string); ok {
			for key, value := range current {
				env[key] = value
			}
		}
		token, _ = r.token.Load().(string)
		env["KANDEV_RUN_TOKEN"], env["KANDEV_API_KEY"] = token, token
	}
	if token == "" {
		return fmt.Errorf("SSH managed subprocess has no run credentials")
	}
	r.token.Store(token)
	r.runEnv.Store(sshManagedRunEnv(env))
	env["KANDEV_API_URL"], env["KANDEV_CLI"] = r.apiURL, r.cli
	return nil
}

func sshManagedRunEnv(env map[string]string) map[string]string {
	result := make(map[string]string)
	for _, key := range sshManagedEnvKeys {
		if value := env[key]; value != "" {
			result[key] = value
		}
	}
	return result
}

func sshManagedProxy(target *url.URL, token string) http.Handler {
	return sshManagedProxyWithToken(target, func() string { return token })
}

func sshManagedProxyWithToken(target *url.URL, token func() string) http.Handler {
	upstream := *target
	upstream.Path, upstream.RawPath, upstream.RawQuery = "", "", ""
	proxy := httputil.NewSingleHostReverseProxy(&upstream)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), []byte("Bearer "+token())) != 1 {
			http.Error(w, "run authentication required", http.StatusUnauthorized)
			return
		}
		if !strings.HasPrefix(r.URL.Path, strings.TrimRight(target.Path, "/")+"/office/") {
			http.Error(w, "Office API only", http.StatusForbidden)
			return
		}
		proxy.ServeHTTP(w, r)
	})
}
