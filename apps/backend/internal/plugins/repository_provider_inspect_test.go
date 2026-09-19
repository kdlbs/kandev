package plugins

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"runtime"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/plugins/pkgtar/pkgtartest"
	"github.com/kandev/kandev/pkg/pluginsdk"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

func TestInspectRepositoryProviderUsesOwnedDeclaredActionAndVerifiedWorkspace(t *testing.T) {
	svc, _, _ := newTestService(t)
	if _, err := svc.Install(t.Context(), repositoryInspectPackage(t, "workspace")); err != nil {
		t.Fatalf("install provider plugin: %v", err)
	}
	request := RepositoryProviderInspectionRequest{
		Provider: "bitbucket", URL: "https://bitbucket.example.test/projects/acme/widgets",
		ProviderScope: "workspace-a", ProviderRepositoryID: "repo-42",
	}
	inspection, err := svc.inspectRepositoryProvider(t.Context(), "workspace-1", request,
		func(_ context.Context, id string, _ pluginDispatchGeneration, action *pluginsdk.PluginActionRequest) (*pluginsdk.PluginActionResponse, error) {
			if id != "kandev-plugin-bitbucket" || action.ActionKey != repositoryInspectActionKey {
				t.Fatalf("unexpected dispatch: id=%q action=%q", id, action.ActionKey)
			}
			if action.Context.WorkspaceID != "workspace-1" {
				t.Fatalf("workspace context = %q", action.Context.WorkspaceID)
			}
			var body map[string]string
			if err := json.Unmarshal(action.Body, &body); err != nil {
				t.Fatalf("decode request body: %v", err)
			}
			if len(body) != 1 || body["url"] != request.URL {
				t.Fatalf("request body = %#v, want only URL", body)
			}
			return &pluginsdk.PluginActionResponse{Body: validInspectionResponse()}, nil
		})
	if err != nil {
		t.Fatalf("inspectRepositoryProvider: %v", err)
	}
	if inspection.ProviderID != "bitbucket" || inspection.ProviderHost != "https://bitbucket.example.test" ||
		inspection.ProviderScope != "workspace-a" || inspection.ProviderRepositoryID != "repo-42" ||
		inspection.OwnerOrProject != "acme" || inspection.Name != "widgets" ||
		inspection.CloneURL != "https://bitbucket.example.test/scm/acme/widgets.git" || inspection.DefaultBranch != "main" {
		t.Fatalf("inspection = %+v", inspection)
	}
}

func TestInspectRepositoryProviderAcceptsDirectDescriptorCompatibilityResponse(t *testing.T) {
	request := RepositoryProviderInspectionRequest{Provider: "bitbucket", URL: "https://bitbucket.example.test/acme/widgets"}
	inspection, err := parseRepositoryProviderInspection(&pluginsdk.PluginActionResponse{Body: directInspectionResponse()}, request)
	if err != nil {
		t.Fatalf("parse direct inspection: %v", err)
	}
	if inspection.ProviderRepositoryID != "repo-42" || inspection.DefaultBranch != "main" {
		t.Fatalf("inspection = %+v", inspection)
	}
}

func TestInspectRepositoryProviderRejectsInvalidContractResponses(t *testing.T) {
	request := RepositoryProviderInspectionRequest{Provider: "bitbucket", URL: "https://bitbucket.example.test/acme/widgets"}
	tests := []struct {
		name string
		body []byte
	}{
		{name: "malformed", body: []byte(`{"repository":`)},
		{name: "provider mismatch", body: []byte(`{"repository":{"provider_id":"github","provider_host":"bitbucket.example.test","provider_repository_id":"repo-42","owner_or_project":"acme","name":"widgets","clone_url":"https://bitbucket.example.test/scm/acme/widgets.git","default_branch":"main"}}`)},
		{name: "credential-bearing clone", body: []byte(`{"repository":{"provider_id":"bitbucket","provider_host":"bitbucket.example.test","provider_repository_id":"repo-42","owner_or_project":"acme","name":"widgets","clone_url":"https://token@bitbucket.example.test/scm/acme/widgets.git","default_branch":"main"}}`)},
		{name: "foreign clone origin", body: []byte(`{"repository":{"provider_id":"bitbucket","provider_host":"bitbucket.example.test","provider_repository_id":"repo-42","owner_or_project":"acme","name":"widgets","clone_url":"https://attacker.example.test/acme/widgets.git","default_branch":"main"}}`)},
		{name: "missing immutable field", body: []byte(`{"repository":{"provider_id":"bitbucket","provider_host":"bitbucket.example.test","provider_repository_id":"repo-42","owner_or_project":"acme","clone_url":"https://bitbucket.example.test/scm/acme/widgets.git","default_branch":"main"}}`)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := parseRepositoryProviderInspection(&pluginsdk.PluginActionResponse{Body: test.body}, request)
			assertRepositoryProviderError(t, err, RepositoryProviderErrorInvalid)
		})
	}

	_, err := parseRepositoryProviderInspection(&pluginsdk.PluginActionResponse{Body: []byte(`{"matched":false}`)}, request)
	assertRepositoryProviderError(t, err, RepositoryProviderErrorNotFound)
}

func TestInspectRepositoryProviderRejectsMissingOwnerAndWrongActionScope(t *testing.T) {
	for _, test := range []struct {
		name string
		pkg  func(*testing.T) *bytes.Buffer
	}{
		{name: "missing owner", pkg: func(t *testing.T) *bytes.Buffer {
			return testPackageWithRepositoryProvider(t, "kandev-plugin-bitbucket", "bitbucket")
		}},
		{name: "wrong action scope", pkg: func(t *testing.T) *bytes.Buffer {
			return repositoryInspectPackage(t, "task")
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			svc, _, _ := newTestService(t)
			if _, err := svc.Install(t.Context(), test.pkg(t)); err != nil {
				t.Fatalf("install provider plugin: %v", err)
			}
			_, err := svc.inspectRepositoryProvider(t.Context(), "workspace-1", RepositoryProviderInspectionRequest{
				Provider: "bitbucket", URL: "https://bitbucket.example.test/acme/widgets",
			}, nil)
			assertRepositoryProviderError(t, err, RepositoryProviderErrorUnavailable)
		})
	}
}

func TestInspectRepositoryProviderBoundsResponseAndTimeout(t *testing.T) {
	request := RepositoryProviderInspectionRequest{Provider: "bitbucket", URL: "https://bitbucket.example.test/acme/widgets"}
	tooLarge := bytes.Repeat([]byte("x"), maxRepositoryInspectResponseSize+1)
	_, err := parseRepositoryProviderInspection(&pluginsdk.PluginActionResponse{Body: tooLarge}, request)
	assertRepositoryProviderError(t, err, RepositoryProviderErrorInvalid)

	svc, _, _ := newTestService(t)
	if _, err := svc.Install(t.Context(), repositoryInspectPackage(t, "workspace")); err != nil {
		t.Fatalf("install provider plugin: %v", err)
	}
	_, err = svc.inspectRepositoryProvider(t.Context(), "workspace-1", request,
		func(ctx context.Context, _ string, _ pluginDispatchGeneration, _ *pluginsdk.PluginActionRequest) (*pluginsdk.PluginActionResponse, error) {
			if _, ok := ctx.Deadline(); !ok {
				return nil, errors.New("inspection context has no deadline")
			}
			return nil, context.DeadlineExceeded
		})
	assertRepositoryProviderError(t, err, RepositoryProviderErrorUnavailable)
}

func TestInspectRepositoryProviderRejectsPinMismatch(t *testing.T) {
	request := RepositoryProviderInspectionRequest{
		Provider: "bitbucket", URL: "https://bitbucket.example.test/acme/widgets",
		ProviderScope: "workspace-b", ProviderRepositoryID: "repo-other",
	}
	_, err := parseRepositoryProviderInspection(&pluginsdk.PluginActionResponse{Body: validInspectionResponse()}, request)
	assertRepositoryProviderError(t, err, RepositoryProviderErrorInvalid)
}

func TestInspectRepositoryProviderRejectsRedirectResponse(t *testing.T) {
	svc, _, _ := newTestService(t)
	if _, err := svc.Install(t.Context(), repositoryInspectPackage(t, "workspace")); err != nil {
		t.Fatalf("install provider plugin: %v", err)
	}
	request := RepositoryProviderInspectionRequest{Provider: "bitbucket", URL: "https://bitbucket.example.test/acme/widgets"}
	_, err := svc.inspectRepositoryProvider(t.Context(), "workspace-1", request,
		func(context.Context, string, pluginDispatchGeneration, *pluginsdk.PluginActionRequest) (*pluginsdk.PluginActionResponse, error) {
			return &pluginsdk.PluginActionResponse{Status: 302, Body: validInspectionResponse()}, nil
		})
	assertRepositoryProviderError(t, err, RepositoryProviderErrorUnavailable)
}

func TestInspectRepositoryProviderLogsSanitizedOutcome(t *testing.T) {
	svc, _, _ := newTestService(t)
	if _, err := svc.Install(t.Context(), repositoryInspectPackage(t, "workspace")); err != nil {
		t.Fatalf("install provider plugin: %v", err)
	}
	core, observed := observer.New(zapcore.InfoLevel)
	log, err := logger.NewFromZap(zap.New(core))
	if err != nil {
		t.Fatalf("create observed logger: %v", err)
	}
	svc.log = log

	request := RepositoryProviderInspectionRequest{
		Provider: "bitbucket", URL: "https://bitbucket.example.test/acme/widgets",
	}
	tests := []struct {
		name        string
		invoke      repositoryActionInvoker
		wantShape   string
		wantFailure string
		wantError   RepositoryProviderErrorCode
	}{
		{
			name: "success",
			invoke: func(context.Context, string, pluginDispatchGeneration, *pluginsdk.PluginActionRequest) (*pluginsdk.PluginActionResponse, error) {
				return &pluginsdk.PluginActionResponse{Body: validInspectionResponse()}, nil
			},
			wantShape:   "nested_descriptor",
			wantFailure: "none",
		},
		{
			name: "invalid descriptor",
			invoke: func(context.Context, string, pluginDispatchGeneration, *pluginsdk.PluginActionRequest) (*pluginsdk.PluginActionResponse, error) {
				return &pluginsdk.PluginActionResponse{Body: []byte(`{"repository":{"provider_id":"bitbucket"}}`)}, nil
			},
			wantShape:   "nested_descriptor",
			wantFailure: "invalid",
			wantError:   RepositoryProviderErrorInvalid,
		},
		{
			name: "provider unavailable",
			invoke: func(context.Context, string, pluginDispatchGeneration, *pluginsdk.PluginActionRequest) (*pluginsdk.PluginActionResponse, error) {
				return nil, context.DeadlineExceeded
			},
			wantShape:   "none",
			wantFailure: "unavailable",
			wantError:   RepositoryProviderErrorUnavailable,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			before := observed.Len()
			_, err := svc.inspectRepositoryProvider(t.Context(), "workspace-1", request, test.invoke)
			if test.wantError == "" {
				if err != nil {
					t.Fatalf("inspectRepositoryProvider: %v", err)
				}
			} else {
				assertRepositoryProviderError(t, err, test.wantError)
			}
			entries := observed.All()
			if len(entries) != before+1 {
				t.Fatalf("log entries = %d, want %d", len(entries), before+1)
			}
			fields := entries[before].ContextMap()
			for key, want := range map[string]string{
				"provider_id":      "bitbucket",
				"plugin_id":        "kandev-plugin-bitbucket",
				"workspace_id":     "workspace-1",
				"response_shape":   test.wantShape,
				"failure_category": test.wantFailure,
			} {
				if got := fmt.Sprint(fields[key]); got != want {
					t.Errorf("%s = %q, want %q", key, got, want)
				}
			}
			if _, ok := fields["duration"]; !ok {
				t.Error("duration field is missing")
			}
			if strings.Contains(entries[before].Message, request.URL) || strings.Contains(fmt.Sprint(fields), request.URL) {
				t.Errorf("inspection log leaked repository URL: %s", request.URL)
			}
		})
	}
}

func assertRepositoryProviderError(t *testing.T, err error, want RepositoryProviderErrorCode) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected repository provider error with code %q", want)
	}
	var providerErr *RepositoryProviderError
	if !errors.As(err, &providerErr) {
		t.Fatalf("error = %v, want RepositoryProviderError", err)
	}
	if providerErr.Code != want {
		t.Fatalf("error code = %q, want %q", providerErr.Code, want)
	}
	if strings.Contains(err.Error(), "token") || strings.Contains(err.Error(), "attacker.example.test") {
		t.Fatalf("error leaked upstream details: %v", err)
	}
}

func validInspectionResponse() []byte {
	return []byte(`{"repository":{"provider_id":"bitbucket","provider_host":"bitbucket.example.test","provider_scope":"workspace-a","provider_repository_id":"repo-42","owner_or_project":"acme","name":"widgets","clone_url":"https://bitbucket.example.test/scm/acme/widgets.git","default_branch":"main"}}`)
}

func directInspectionResponse() []byte {
	return []byte(`{"provider_id":"bitbucket","provider_host":"bitbucket.example.test","provider_repository_id":"repo-42","owner_or_project":"acme","name":"widgets","clone_url":"https://bitbucket.example.test/scm/acme/widgets.git","default_branch":"main"}`)
}

func repositoryInspectPackage(t *testing.T, scope string) *bytes.Buffer {
	t.Helper()
	manifestYAML := fmt.Sprintf(`
id: kandev-plugin-bitbucket
api_version: 1
version: "1.0.0"
display_name: Bitbucket
repository_providers: [bitbucket]
actions:
  - key: repositories.inspect
    scope: %s
    max_body_bytes: 16384
runtime:
  type: binary
  executables:
    %s-%s: server/plugin
`, scope, runtime.GOOS, runtime.GOARCH)
	var buffer bytes.Buffer
	if err := pkgtartest.WritePackage(&buffer, map[string][]byte{
		"manifest.yaml": []byte(manifestYAML),
		"server/plugin": []byte("#!/bin/sh\necho fake\n"),
	}); err != nil {
		t.Fatalf("WritePackage: %v", err)
	}
	return &buffer
}
