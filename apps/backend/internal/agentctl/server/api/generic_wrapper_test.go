package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/agentctl/server/process"
	"github.com/kandev/kandev/internal/common/launchwrapper"
)

func TestGenericWrapperRequiresAuthenticatedControl(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX execution fixture")
	}
	for _, tc := range []struct {
		name, token, header string
		status              int
		spawned             bool
	}{
		{name: "disabled auth", status: http.StatusForbidden},
		{name: "forged credential", token: "control-secret", header: "Bearer wrong", status: http.StatusUnauthorized},
		{name: "authenticated", token: "control-secret", header: "Bearer control-secret", status: http.StatusOK, spawned: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			receipt := filepath.Join(dir, "receipt")
			wrapper := filepath.Join(dir, "unrelated-wrapper")
			child := filepath.Join(dir, "mock-agent")
			require.NoError(t, os.WriteFile(wrapper, []byte("#!/bin/sh\nprintf launched > \"$RECEIPT\"\nexit 1\n"), 0700))
			require.NoError(t, os.WriteFile(child, []byte("#!/bin/sh\nexit 1\n"), 0700))
			t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
			log := newTestLogger()
			cfg := &config.InstanceConfig{WorkDir: dir, AuthToken: tc.token}
			server := NewServer(cfg, process.NewManager(cfg, log), nil, nil, log)
			body, err := json.Marshal(map[string]any{"agent_id": "mock-agent", "inference_config": map[string]any{
				"work_dir": dir, "command": []string{"mock-agent"}, "env": map[string]string{"RECEIPT": receipt},
				"launch_wrapper": launchwrapper.Profile([]string{wrapper}),
			}})
			require.NoError(t, err)
			req := httptest.NewRequest(http.MethodPost, "/api/v1/inference/probe", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", tc.header)
			response := httptest.NewRecorder()
			server.Router().ServeHTTP(response, req)
			require.Equal(t, tc.status, response.Code, response.Body.String())
			_, err = os.Stat(receipt)
			if tc.spawned {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, os.ErrNotExist)
			}
		})
	}
}

func TestGenericWrapperCapabilityMatchesInstanceAndControl(t *testing.T) {
	cfg := &config.InstanceConfig{WorkDir: t.TempDir(), AuthToken: "test-control"}
	log := newTestLogger()
	server := NewServer(cfg, process.NewManager(cfg, log), nil, nil, log)
	for _, auth := range []string{"", "Bearer test-control"} {
		t.Run(auth, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/identity", nil)
			if auth != "" {
				req.Header.Set("Authorization", auth)
			}
			response := httptest.NewRecorder()
			server.Router().ServeHTTP(response, req)
			if auth == "" {
				require.Equal(t, http.StatusUnauthorized, response.Code)
				require.NotContains(t, response.Body.String(), launchwrapper.Capability)
				return
			}
			require.Equal(t, http.StatusOK, response.Code)
			var identity struct {
				Capabilities []string `json:"capabilities"`
			}
			require.NoError(t, json.Unmarshal(response.Body.Bytes(), &identity))
			require.Contains(t, identity.Capabilities, launchwrapper.Capability)
			require.Equal(t, SurvivalCapabilities, identity.Capabilities)
		})
	}
}
