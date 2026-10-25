package hostutility

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/registry"
	agentctlclient "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	agentctlutil "github.com/kandev/kandev/internal/agentctl/server/utility"
	"go.uber.org/zap"
)

func TestProbeIsolatedWithCommandKeepsSubprocessStateInsidePrivateRoot(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("ACP fixture uses a POSIX shell")
	}
	root := t.TempDir()
	outside := t.TempDir()
	binDir := t.TempDir()
	outsideConfig := filepath.Join(outside, "user-opencode.json")
	outsideDB := filepath.Join(outside, "user-opencode.db")
	outsideState := filepath.Join(outside, "state")
	for path, value := range map[string]string{
		outsideConfig: "user config sentinel\n",
		outsideDB:     "user database sentinel\n",
	} {
		if err := os.WriteFile(path, []byte(value), 0o600); err != nil {
			t.Fatalf("write user state fixture: %v", err)
		}
	}
	if err := os.MkdirAll(outsideState, 0o700); err != nil {
		t.Fatalf("create outside state fixture: %v", err)
	}
	commandPath := filepath.Join(binDir, "opencode")
	command := `#!/bin/sh
if [ "$1" = models ]; then echo "opencode/test"; exit 0; fi
config_dir="${OPENCODE_CONFIG_DIR:-$XDG_CONFIG_HOME/opencode}"
data_dir="${OPENCODE_DATA_DIR:-$XDG_DATA_HOME/opencode}"
cache_dir="${OPENCODE_CACHE_DIR:-$XDG_CACHE_HOME/opencode}"
state_dir="${OPENCODE_STATE_DIR:-$XDG_STATE_HOME/opencode}"
config_file="${OPENCODE_CONFIG:-$config_dir/opencode.json}"
db_file="${OPENCODE_DB:-$data_dir/opencode.db}"
mkdir -p "$config_dir" "$data_dir" "$cache_dir" "$state_dir" "$(dirname "$config_file")" "$(dirname "$db_file")"
{
  printf 'HOME=%s\n' "$HOME"
  printf 'XDG_CONFIG_HOME=%s\n' "$XDG_CONFIG_HOME"
  printf 'XDG_DATA_HOME=%s\n' "$XDG_DATA_HOME"
  printf 'XDG_CACHE_HOME=%s\n' "$XDG_CACHE_HOME"
  printf 'XDG_STATE_HOME=%s\n' "$XDG_STATE_HOME"
  printf 'OPENCODE_CONFIG=%s\n' "$config_file"
  printf 'OPENCODE_DB=%s\n' "$db_file"
  printf 'OPENCODE_CONFIG_DIR=%s\n' "${OPENCODE_CONFIG_DIR-unset}"
  printf 'OPENCODE_DATA_DIR=%s\n' "${OPENCODE_DATA_DIR-unset}"
  printf 'OPENCODE_CACHE_DIR=%s\n' "${OPENCODE_CACHE_DIR-unset}"
  printf 'OPENCODE_STATE_DIR=%s\n' "${OPENCODE_STATE_DIR-unset}"
  printf 'OPENCODE_CONFIG_CONTENT=%s\n' "${OPENCODE_CONFIG_CONTENT-unset}"
  printf 'NPM_CONFIG_CACHE=%s\n' "${NPM_CONFIG_CACHE-unset}"
  printf 'npm_config_cache=%s\n' "${npm_config_cache-unset}"
  printf 'NPM_CONFIG_USERCONFIG=%s\n' "${NPM_CONFIG_USERCONFIG-unset}"
  printf 'npm_config_userconfig=%s\n' "${npm_config_userconfig-unset}"
} > "$state_dir/effective.env"
printf 'candidate config write\n' >> "$config_file"
printf 'candidate database write\n' >> "$db_file"
read -r INITIALIZE
INITIALIZE_ID=$(printf '%s' "$INITIALIZE" | sed -n 's/.*"id":\([^,}]*\).*/\1/p')
printf '{"jsonrpc":"2.0","id":%s,"result":{"protocolVersion":1,"agentCapabilities":{}}}\n' "$INITIALIZE_ID"
read -r NEW_SESSION
SESSION_ID=$(printf '%s' "$NEW_SESSION" | sed -n 's/.*"id":\([^,}]*\).*/\1/p')
printf '{"jsonrpc":"2.0","id":%s,"result":{"sessionId":"probe","configOptions":[{"type":"select","id":"model","name":"Model","category":"model","currentValue":"opencode/test","options":[{"value":"opencode/test","name":"test"}]}]}}\n' "$SESSION_ID"
cat >/dev/null
`
	if err := os.WriteFile(commandPath, []byte(command), 0o755); err != nil {
		t.Fatalf("write OpenCode fixture: %v", err)
	}
	npmPath := filepath.Join(binDir, "npm")
	npm := "#!/bin/sh\ncase \"$3\" in\n  cache) printf '%s\\n' '" + filepath.Join(outside, "npm-cache") + "' ;;\n  userconfig) printf '%s\\n' '" + filepath.Join(outside, "npmrc") + "' ;;\n  *) exit 2 ;;\nesac\n"
	if err := os.WriteFile(npmPath, []byte(npm), 0o755); err != nil {
		t.Fatalf("write npm config fixture: %v", err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	userPaths := map[string]string{
		"HOME":                    filepath.Join(outside, "home"),
		"XDG_CONFIG_HOME":         filepath.Join(outside, "xdg-config"),
		"XDG_DATA_HOME":           filepath.Join(outside, "xdg-data"),
		"XDG_CACHE_HOME":          filepath.Join(outside, "xdg-cache"),
		"XDG_STATE_HOME":          outsideState,
		"OPENCODE_CONFIG":         outsideConfig,
		"OPENCODE_DB":             outsideDB,
		"OPENCODE_CONFIG_DIR":     filepath.Join(outside, "config-dir"),
		"OPENCODE_DATA_DIR":       filepath.Join(outside, "data-dir"),
		"OPENCODE_CACHE_DIR":      filepath.Join(outside, "cache-dir"),
		"OPENCODE_STATE_DIR":      filepath.Join(outside, "state-dir"),
		"OPENCODE_CONFIG_CONTENT": `{"provider":{"sentinel":"user"}}`,
		"NPM_CONFIG_CACHE":        filepath.Join(outside, "inherited-npm-cache"),
		"npm_config_cache":        filepath.Join(outside, "inherited-lower-npm-cache"),
		"NPM_CONFIG_USERCONFIG":   filepath.Join(outside, "inherited-npmrc"),
		"npm_config_userconfig":   filepath.Join(outside, "inherited-lower-npmrc"),
	}
	for key, value := range userPaths {
		t.Setenv(key, value)
	}

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/health", func(c *gin.Context) { c.Status(http.StatusOK) })
	agentctlutil.NewHandler("", zap.NewNop()).RegisterRoutes(router.Group("/api/v1"))
	server := httptest.NewServer(router)
	t.Cleanup(server.Close)
	log := newTestLogger(t)
	host, port := serverHostPort(t, server)
	reg := registry.NewRegistry(log)
	openCode := agents.NewOpenCodeACP()
	if err := reg.Register(openCode); err != nil {
		t.Fatalf("register OpenCode: %v", err)
	}
	manager := NewManager(reg, host, port, nil, log)
	manager.instances[openCode.ID()] = &instance{
		agentType: openCode.ID(),
		workDir:   t.TempDir(),
		client:    agentctlclient.NewClient(host, port, log),
	}
	manager.parentTmpDir = t.TempDir()

	caps, err := manager.ProbeIsolatedWithCommand(context.Background(), openCode.ID(), agents.NewCommand(
		"opencode", "acp", "--print-logs", "--log-level", "error",
	), root)
	if err != nil {
		t.Fatalf("ProbeIsolatedWithCommand: %v", err)
	}
	if caps.Status != StatusOK {
		t.Fatalf("isolated probe status = %q, error %q", caps.Status, caps.Error)
	}

	reportPath := filepath.Join(root, "state", "opencode", "effective.env")
	report, err := os.ReadFile(reportPath)
	if err != nil {
		t.Fatalf("read child environment report under isolated root: %v", err)
	}
	got := make(map[string]string)
	for _, line := range strings.Split(strings.TrimSpace(string(report)), "\n") {
		key, value, ok := strings.Cut(line, "=")
		if ok {
			got[key] = value
		}
	}
	want := map[string]string{
		"HOME":                    filepath.Join(root, "home"),
		"XDG_CONFIG_HOME":         filepath.Join(root, "config"),
		"XDG_DATA_HOME":           filepath.Join(root, "data"),
		"XDG_CACHE_HOME":          filepath.Join(root, "cache"),
		"XDG_STATE_HOME":          filepath.Join(root, "state"),
		"OPENCODE_CONFIG":         filepath.Join(root, "config", "opencode.json"),
		"OPENCODE_DB":             filepath.Join(root, "data", "opencode", "opencode.db"),
		"OPENCODE_CONFIG_DIR":     "unset",
		"OPENCODE_DATA_DIR":       "unset",
		"OPENCODE_CACHE_DIR":      "unset",
		"OPENCODE_STATE_DIR":      "unset",
		"OPENCODE_CONFIG_CONTENT": "unset",
		"NPM_CONFIG_CACHE":        filepath.Join(outside, "npm-cache"),
		"npm_config_cache":        filepath.Join(outside, "npm-cache"),
		"NPM_CONFIG_USERCONFIG":   filepath.Join(outside, "npmrc"),
		"npm_config_userconfig":   filepath.Join(outside, "npmrc"),
	}
	for key, value := range want {
		if got[key] != value {
			t.Errorf("child %s = %q, want %q", key, got[key], value)
		}
	}
	for path, sentinel := range map[string]string{
		outsideConfig: "user config sentinel\n",
		outsideDB:     "user database sentinel\n",
	} {
		contents, err := os.ReadFile(path)
		if err != nil || string(contents) != sentinel {
			t.Errorf("user state %q changed to %q, err=%v", path, contents, err)
		}
	}
	for _, path := range []string{
		filepath.Join(root, "config", "opencode.json"),
		filepath.Join(root, "data", "opencode", "opencode.db"),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Errorf("isolated state file %q missing: %v", path, err)
		}
	}
}
