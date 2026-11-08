package client

import (
	"context"
	"errors"
	"github.com/kandev/kandev/internal/agentctl/server/utility"
	"github.com/kandev/kandev/internal/common/logger"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"
)

type stopProofConsumer struct {
	calls int
	fail  bool
}

func (c *stopProofConsumer) Execute(context.Context, *Client, *utility.PromptRequest, InferenceIdentity) (*utility.PromptResponse, error) {
	return nil, nil
}
func (c *stopProofConsumer) ObserveInferenceStop(_ context.Context, s *utility.AttemptSnapshot) error {
	c.calls++
	if s.InstanceID != "instance" {
		return errors.New("wrong proof")
	}
	if c.fail {
		return errors.New("settlement unavailable")
	}
	return nil
}
func TestInferenceStopProofClientExactAndLegacyDisposition(t *testing.T) {
	for _, tc := range []struct {
		name    string
		code    int
		body    string
		fail    bool
		calls   int
		wantErr bool
	}{
		{"exact", 200, `{"inference_attempts":{"instance_id":"instance","known":true,"attempts":[]}}`, false, 1, false},
		{"settlement_failure", 200, `{"inference_attempts":{"instance_id":"instance","known":true,"attempts":[]}}`, true, 1, true},
		{"foreign_instance", 200, `{"inference_attempts":{"instance_id":"other","known":true,"attempts":[]}}`, false, 0, false},
		{"legacy", 200, `{"message":"stopped"}`, false, 0, false},
		{"malformed", 200, `{`, false, 0, false},
		{"already_absent", 404, `{"inference_attempts":{"instance_id":"instance","known":true,"attempts":[]}}`, false, 0, false},
		{"no_content", 204, ``, false, 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "DELETE" || r.URL.Path != "/api/v1/instances/instance" {
					t.Errorf("wrong delete %s %s", r.Method, r.URL.Path)
				}
				w.WriteHeader(tc.code)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer server.Close()
			u, _ := url.Parse(server.URL)
			port, _ := strconv.Atoi(u.Port())
			client := NewControlClient(u.Hostname(), port, logger.Default())
			defer client.Close()
			consumer := &stopProofConsumer{fail: tc.fail}
			err := client.DeleteInstance(WithInferenceStopObserver(context.Background(), consumer), "instance")
			if consumer.calls != tc.calls || (err != nil) != tc.wantErr {
				t.Fatalf("proof delivery calls=%d err=%v", consumer.calls, err)
			}
		})
	}
}
