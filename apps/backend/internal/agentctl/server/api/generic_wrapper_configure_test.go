package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/agentctl/server/process"
)

func TestGenericWrapperConfigureEnvelope(t *testing.T) {
	for _, tc := range []struct {
		name, envelope string
		want           int
	}{
		{"valid", `{"version":1,"source":"profile","argv":["wrapper","{{kandev.work_dir}}"]}`, http.StatusOK},
		{"null", `null`, http.StatusBadRequest},
		{"unknown version", `{"version":2,"source":"profile","argv":["wrapper"]}`, http.StatusBadRequest},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &config.InstanceConfig{WorkDir: t.TempDir(), AuthToken: "test-control"}
			log := newTestLogger()
			manager := process.NewManager(cfg, log)
			server := NewServer(cfg, manager, nil, nil, log)
			body := []byte(`{"command":"mock-agent","agent_args":["mock-agent"],"launch_wrapper":` + tc.envelope + `}`)
			req := httptest.NewRequest(http.MethodPost, "/api/v1/agent/configure", bytes.NewReader(body))
			req.Header.Set("Authorization", "Bearer test-control")
			req.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			server.Router().ServeHTTP(response, req)
			require.Equal(t, tc.want, response.Code, response.Body.String())
			if tc.want == http.StatusOK {
				data, err := json.Marshal(cfg)
				require.NoError(t, err)
				var persisted map[string]json.RawMessage
				require.NoError(t, json.Unmarshal(data, &persisted))
				require.JSONEq(t, tc.envelope, string(persisted["launch_wrapper"]))
			} else {
				require.Empty(t, cfg.AgentArgs, "rejected configuration must leave state unchanged")
			}
		})
	}
}
