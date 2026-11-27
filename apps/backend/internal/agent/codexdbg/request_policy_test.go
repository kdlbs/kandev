package codexdbg

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/kandev/kandev/pkg/codexappserver"
)

func TestRequestPolicyDeclinesByDefaultAndRejectsUnknownMethods(t *testing.T) {
	policy := NewRequestPolicy(nil)
	response, err := policy.Handle(context.Background(), codexappserver.ServerRequest{Method: "item/commandExecution/requestApproval"})
	if err != nil {
		t.Fatalf("known request: %v", err)
	}
	var approval struct {
		Decision string `json:"decision"`
	}
	if err := json.Unmarshal(response.(json.RawMessage), &approval); err != nil {
		t.Fatal(err)
	}
	if approval.Decision != "decline" {
		t.Fatalf("default decision = %q, want decline", approval.Decision)
	}

	policy = NewRequestPolicy(map[string]json.RawMessage{
		"unknown/request": json.RawMessage(`{"decision":"accept"}`),
	})
	_, err = policy.Handle(context.Background(), codexappserver.ServerRequest{Method: "unknown/request"})
	var rpcErr *codexappserver.RPCError
	if !errors.As(err, &rpcErr) || rpcErr.Code != -32601 {
		t.Fatalf("unknown request error = %v, want method-not-found", err)
	}
}

func TestAnswerFileValidatesKnownRequestMethods(t *testing.T) {
	path := filepath.Join(t.TempDir(), "answers.json")
	if err := os.WriteFile(path, []byte(`{"item/commandExecution/requestApproval":{"decision":"accept"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	answers, err := LoadAnswerFile(path)
	if err != nil {
		t.Fatalf("load answer file: %v", err)
	}
	policy := NewRequestPolicy(answers)
	response, err := policy.Handle(context.Background(), codexappserver.ServerRequest{Method: "item/commandExecution/requestApproval"})
	if err != nil {
		t.Fatal(err)
	}
	if string(response.(json.RawMessage)) != `{"decision":"accept"}` {
		t.Fatalf("explicit answer = %s", response)
	}

	if err := os.WriteFile(path, []byte(`{"unrecognized/request":{"decision":"accept"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadAnswerFile(path); err == nil {
		t.Fatal("answer file accepted an unknown request method")
	}
}

func TestUserInputAnswersMatchRequestQuestionsAndOptions(t *testing.T) {
	request := codexappserver.ServerRequest{
		Method: codexappserver.ServerRequestToolUserInput,
		Params: json.RawMessage(`{"questions":[{"id":"q-mode","options":[{"label":"Fast","description":"Quick"},{"label":"Careful","description":"Thorough"}]},{"id":"q-name","isOther":true,"options":[{"label":"Default","description":"Use the default"}]}]}`),
	}
	tests := []struct {
		name    string
		answer  string
		wantErr bool
	}{
		{
			name:   "offered selections and other text",
			answer: `{"answers":{"q-mode":{"answers":["Fast"]},"q-name":{"answers":["custom name"]}}}`,
		},
		{
			name:    "unknown question ID",
			answer:  `{"answers":{"q-missing":{"answers":["Fast"]},"q-name":{"answers":["custom name"]}}}`,
			wantErr: true,
		},
		{
			name:    "missing question answer",
			answer:  `{"answers":{"q-mode":{"answers":["Fast"]}}}`,
			wantErr: true,
		},
		{
			name:    "unoffered choice",
			answer:  `{"answers":{"q-mode":{"answers":["Quick"]},"q-name":{"answers":["custom name"]}}}`,
			wantErr: true,
		},
		{
			name:    "multiple choices for a single question",
			answer:  `{"answers":{"q-mode":{"answers":["Fast","Careful"]},"q-name":{"answers":["custom name"]}}}`,
			wantErr: true,
		},
		{
			name:    "malformed response shape",
			answer:  `{"answers":{"q-mode":{"answers":"Fast"},"q-name":{"answers":["custom name"]}}}`,
			wantErr: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			policy := NewRequestPolicy(map[string]json.RawMessage{
				codexappserver.ServerRequestToolUserInput: json.RawMessage(test.answer),
			})
			_, err := policy.Handle(context.Background(), request)
			if (err != nil) != test.wantErr {
				t.Fatalf("Handle error = %v, wantErr %v", err, test.wantErr)
			}
		})
	}
}

func TestServerRequestCoverage(t *testing.T) {
	methods := codexappserver.ServerRequestMethodsV0154()
	seen := make(map[string]struct{}, len(methods))
	policy := NewRequestPolicy(nil)
	for _, method := range methods {
		if _, duplicate := seen[method]; duplicate {
			t.Fatalf("server request inventory contains duplicate method %q", method)
		}
		seen[method] = struct{}{}
		_, safeDefault := requestDefaults[method]
		_, explicitReject := requestRejections[method]
		if safeDefault == explicitReject {
			t.Errorf("server request %q must have exactly one policy", method)
			continue
		}
		if explicitReject {
			_, err := policy.Handle(context.Background(), codexappserver.ServerRequest{Method: method})
			var rpcErr *codexappserver.RPCError
			if !errors.As(err, &rpcErr) || rpcErr.Code != -32601 {
				t.Errorf("explicitly rejected request %q returned %v, want method-not-found", method, err)
			}
		}
	}
	if len(seen) != len(requestDefaults)+len(requestRejections) {
		t.Fatalf("classified %d methods, pinned inventory has %d", len(requestDefaults)+len(requestRejections), len(seen))
	}
	for method := range requestDefaults {
		if _, exists := seen[method]; !exists {
			t.Errorf("safe default for %q is absent from the pinned inventory", method)
		}
	}
	for method := range requestRejections {
		if _, exists := seen[method]; !exists {
			t.Errorf("explicit rejection for %q is absent from the pinned inventory", method)
		}
	}
}
