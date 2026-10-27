package client

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/server/utility"
	"github.com/kandev/kandev/internal/common/launchwrapper"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/common/sandbox"
	"github.com/stretchr/testify/require"
)

func TestSandboxLaunchRequiresPeerCapabilityBeforeUtilityDispatch(t *testing.T) {
	content := []byte("fixture runtime")
	digest := sha256.Sum256(content)
	files, err := sandbox.MaterializeRuntime(context.Background(), t.TempDir(), content, hex.EncodeToString(digest[:]), nil, nil)
	require.NoError(t, err)
	defer func() { require.NoError(t, files.Close()) }()
	launch := &sandbox.Launch{Version: 1, AttemptID: "attempt-1", Generation: 1, PolicyDigest: strings.Repeat("a", 64), PackageDigest: strings.Repeat("b", 64), Root: files.Root, Prefix: []string{files.Executable}, Digests: files.Digests}
	for _, supported := range []bool{false, true} {
		t.Run(strconv.FormatBool(supported), func(t *testing.T) {
			dispatched := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/identity" {
					capabilities := []string{}
					if supported {
						capabilities = append(capabilities, sandbox.LaunchCapability)
					}
					require.NoError(t, json.NewEncoder(w).Encode(map[string]any{"capabilities": capabilities}))
					return
				}
				dispatched++
				var request struct {
					InferenceConfig *utility.InferenceConfigDTO `json:"inference_config"`
					Sandbox         *sandbox.Launch             `json:"sandbox"`
				}
				require.NoError(t, json.NewDecoder(r.Body).Decode(&request))
				if r.URL.Path == "/api/v1/agent/configure" {
					require.NotNil(t, request.Sandbox)
					require.Equal(t, launch.AttemptID, request.Sandbox.AttemptID)
				} else {
					require.NotNil(t, request.InferenceConfig.Sandbox)
					require.Equal(t, launch.AttemptID, request.InferenceConfig.Sandbox.AttemptID)
				}
				require.NoError(t, json.NewEncoder(w).Encode(utility.PromptResponse{Success: true}))
			}))
			defer server.Close()
			host, portText, err := net.SplitHostPort(server.Listener.Addr().String())
			require.NoError(t, err)
			port, err := strconv.Atoi(portText)
			require.NoError(t, err)
			log, err := logger.NewLogger(logger.LoggingConfig{Level: "error"})
			require.NoError(t, err)
			client := NewClient(host, port, log)
			cfg := &utility.InferenceConfigDTO{Sandbox: launch}
			_, promptErr := client.InferencePrompt(context.Background(), &utility.PromptRequest{InferenceConfig: cfg})
			_, probeErr := client.Probe(context.Background(), &utility.ProbeRequest{InferenceConfig: cfg})
			configureErr := client.ConfigureAgentWithSandbox(context.Background(), "fixture", []string{"fixture"}, nil, "", nil, launch, launchwrapper.Spec{})
			if supported {
				require.NoError(t, configureErr)
				require.NoError(t, promptErr)
				require.NoError(t, probeErr)
				require.Equal(t, 3, dispatched)
			} else {
				require.Error(t, configureErr)
				require.Error(t, promptErr)
				require.Error(t, probeErr)
				require.Zero(t, dispatched)
			}
		})
	}
}

func TestSandboxOwnerResponseBindsExpectedAttempt(t *testing.T) {
	for _, kind := range []string{"valid", "wrong-attempt", "missing-birth", "server-error"} {
		t.Run(kind, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/identity" {
					require.NoError(t, json.NewEncoder(w).Encode(map[string]any{"capabilities": []string{sandbox.LaunchCapability}}))
					return
				}
				require.Equal(t, "/api/v1/agent/sandbox/owner", r.URL.Path)
				if kind == "server-error" {
					w.WriteHeader(500)
					_, _ = w.Write([]byte("private-profile-contents"))
					return
				}
				id := "attempt"
				birth := "birth"
				if kind == "wrong-attempt" {
					id = "foreign"
				}
				if kind == "missing-birth" {
					birth = ""
				}
				require.NoError(t, json.NewEncoder(w).Encode(map[string]any{"attempt_id": id, "generation": uint64(2), "policy_digest": strings.Repeat("a", 64), "supervisor": map[string]any{"pid": 100, "group_id": 100, "session_id": 100, "birth_token": birth}}))
			}))
			defer server.Close()
			host, portText, err := net.SplitHostPort(server.Listener.Addr().String())
			require.NoError(t, err)
			port, err := strconv.Atoi(portText)
			require.NoError(t, err)
			log, err := logger.NewLogger(logger.LoggingConfig{Level: "error"})
			require.NoError(t, err)
			client := NewClient(host, port, log)
			result, err := client.SandboxOwner(t.Context(), "attempt", 2, strings.Repeat("a", 64))
			if kind == "valid" {
				require.NoError(t, err)
				require.Equal(t, 100, result.Supervisor.PID)
			} else {
				require.Error(t, err)
				require.NotContains(t, err.Error(), "private-profile-contents")
			}
		})
	}
}
