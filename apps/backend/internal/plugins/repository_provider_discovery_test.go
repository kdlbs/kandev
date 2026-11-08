package plugins

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/plugins/pkgtar/pkgtartest"
	"github.com/kdlbs/kandev/pluginsdk"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

const discoveryTestURL = "https://vcs.example.test/projects/acme/widgets"

type discoveryInvocation struct {
	pluginID string
	provider string
}

// discoveryInvoker returns one canned inspection response per plugin and
// records the verified workspace each candidate received.
func discoveryInvoker(
	t *testing.T, responses map[string]func() (*pluginsdk.PluginActionResponse, error),
) (repositoryActionInvoker, *[]discoveryInvocation) {
	t.Helper()
	var mu sync.Mutex
	calls := make([]discoveryInvocation, 0, len(responses))
	return func(_ context.Context, id string, _ pluginDispatchGeneration, action *pluginsdk.PluginActionRequest) (*pluginsdk.PluginActionResponse, error) {
		if action.ActionKey != repositoryInspectActionKey {
			t.Errorf("action key = %q, want %q", action.ActionKey, repositoryInspectActionKey)
		}
		if action.Context.WorkspaceID != "workspace-1" {
			t.Errorf("workspace context = %q", action.Context.WorkspaceID)
		}
		var body map[string]string
		if err := json.Unmarshal(action.Body, &body); err != nil {
			t.Errorf("decode request body: %v", err)
		}
		if len(body) != 1 || body["url"] != discoveryTestURL {
			t.Errorf("request body = %#v, want only the URL", body)
		}
		mu.Lock()
		calls = append(calls, discoveryInvocation{pluginID: id, provider: action.ActionKey})
		mu.Unlock()
		respond, found := responses[id]
		if !found {
			return nil, fmt.Errorf("unexpected dispatch to %q", id)
		}
		return respond()
	}, &calls
}

func discoveryDescriptor(provider, scope, repositoryID string) []byte {
	return []byte(fmt.Sprintf(
		`{"repository":{"provider_id":%q,"provider_host":"vcs.example.test","provider_scope":%q,`+
			`"provider_repository_id":%q,"owner_or_project":"acme","name":"widgets",`+
			`"clone_url":"https://vcs.example.test/acme/widgets.git","default_branch":"main"}}`,
		provider, scope, repositoryID,
	))
}

func TestDiscoverRepositoryProviders(t *testing.T) {
	t.Run("bitbucket forgejo and future providers share one discovery path", func(t *testing.T) {
		svc, _, _ := newTestService(t)
		installRepositoryProviderPlugin(t, svc, "kandev-plugin-bitbucket", "bitbucket")
		installRepositoryProviderPlugin(t, svc, "kandev-plugin-forgejo", "forgejo")
		installRepositoryProviderPlugin(t, svc, "kandev-plugin-acme", "acme-vcs")
		invoke, calls := discoveryInvoker(t, map[string]func() (*pluginsdk.PluginActionResponse, error){
			"kandev-plugin-bitbucket": func() (*pluginsdk.PluginActionResponse, error) {
				return &pluginsdk.PluginActionResponse{Body: discoveryDescriptor("bitbucket", "workspace-a", "repo-1")}, nil
			},
			"kandev-plugin-forgejo": func() (*pluginsdk.PluginActionResponse, error) {
				return &pluginsdk.PluginActionResponse{Body: discoveryDescriptor("forgejo", "workspace-b", "repo-2")}, nil
			},
			"kandev-plugin-acme": func() (*pluginsdk.PluginActionResponse, error) {
				return &pluginsdk.PluginActionResponse{Body: discoveryDescriptor("acme-vcs", "workspace-c", "repo-3")}, nil
			},
		})

		result, err := svc.discoverRepositoryProviders(t.Context(), "workspace-1",
			RepositoryProviderDiscoveryRequest{URL: discoveryTestURL}, invoke)
		if err != nil {
			t.Fatalf("discoverRepositoryProviders: %v", err)
		}
		if result.Failure != nil {
			t.Fatalf("failure = %v, want none", result.Failure)
		}
		if len(result.Candidates) != 3 {
			t.Fatalf("candidates = %+v, want all three providers", result.Candidates)
		}
		got := make([]string, 0, len(result.Candidates))
		for _, candidate := range result.Candidates {
			got = append(got, candidate.ProviderID+"/"+candidate.ProviderRepositoryID+"/"+candidate.ProviderScope)
		}
		want := []string{"acme-vcs/repo-3/workspace-c", "bitbucket/repo-1/workspace-a", "forgejo/repo-2/workspace-b"}
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Fatalf("candidates = %v, want deterministic plugin order %v", got, want)
		}
		if len(*calls) != 3 {
			t.Fatalf("inspections = %d, want one per eligible provider", len(*calls))
		}
	})

	t.Run("ordinary no-match returns an empty result without failure", func(t *testing.T) {
		svc, _, _ := newTestService(t)
		installRepositoryProviderPlugin(t, svc, "kandev-plugin-bitbucket", "bitbucket")
		invoke, _ := discoveryInvoker(t, map[string]func() (*pluginsdk.PluginActionResponse, error){
			"kandev-plugin-bitbucket": func() (*pluginsdk.PluginActionResponse, error) {
				return &pluginsdk.PluginActionResponse{Body: []byte(`{"matched":false}`)}, nil
			},
		})
		result, err := svc.discoverRepositoryProviders(t.Context(), "workspace-1",
			RepositoryProviderDiscoveryRequest{URL: discoveryTestURL}, invoke)
		if err != nil {
			t.Fatalf("discoverRepositoryProviders: %v", err)
		}
		if len(result.Candidates) != 0 || result.Failure != nil {
			t.Fatalf("result = %+v, want an empty no-match", result)
		}
	})

	t.Run("invalid descriptor is a typed candidate failure", func(t *testing.T) {
		svc, _, _ := newTestService(t)
		installRepositoryProviderPlugin(t, svc, "kandev-plugin-bitbucket", "bitbucket")
		invoke, _ := discoveryInvoker(t, map[string]func() (*pluginsdk.PluginActionResponse, error){
			"kandev-plugin-bitbucket": func() (*pluginsdk.PluginActionResponse, error) {
				return &pluginsdk.PluginActionResponse{Body: []byte(`{"repository":{"provider_id":"bitbucket"}}`)}, nil
			},
		})
		result, err := svc.discoverRepositoryProviders(t.Context(), "workspace-1",
			RepositoryProviderDiscoveryRequest{URL: discoveryTestURL}, invoke)
		if err != nil {
			t.Fatalf("discoverRepositoryProviders: %v", err)
		}
		if len(result.Candidates) != 0 || result.Failure == nil || result.Failure.Code != RepositoryProviderErrorInvalid {
			t.Fatalf("result = %+v, want an invalid candidate failure", result)
		}
	})

	t.Run("a provider failure does not replace another provider's match", func(t *testing.T) {
		svc, _, _ := newTestService(t)
		installRepositoryProviderPlugin(t, svc, "kandev-plugin-bitbucket", "bitbucket")
		installRepositoryProviderPlugin(t, svc, "kandev-plugin-forgejo", "forgejo")
		invoke, _ := discoveryInvoker(t, map[string]func() (*pluginsdk.PluginActionResponse, error){
			"kandev-plugin-bitbucket": func() (*pluginsdk.PluginActionResponse, error) {
				return &pluginsdk.PluginActionResponse{Status: 503}, nil
			},
			"kandev-plugin-forgejo": func() (*pluginsdk.PluginActionResponse, error) {
				return &pluginsdk.PluginActionResponse{Body: discoveryDescriptor("forgejo", "workspace-b", "repo-2")}, nil
			},
		})
		result, err := svc.discoverRepositoryProviders(t.Context(), "workspace-1",
			RepositoryProviderDiscoveryRequest{URL: discoveryTestURL}, invoke)
		if err != nil {
			t.Fatalf("discoverRepositoryProviders: %v", err)
		}
		if len(result.Candidates) != 1 || result.Candidates[0].ProviderID != "forgejo" {
			t.Fatalf("candidates = %+v, want the successful forgejo match", result.Candidates)
		}
		if result.Failure == nil || result.Failure.Code != RepositoryProviderErrorUnavailable {
			t.Fatalf("failure = %v, want unavailable", result.Failure)
		}
	})

	t.Run("failure severity is deterministic", func(t *testing.T) {
		svc, _, _ := newTestService(t)
		installRepositoryProviderPlugin(t, svc, "kandev-plugin-bitbucket", "bitbucket")
		installRepositoryProviderPlugin(t, svc, "kandev-plugin-forgejo", "forgejo")
		invoke, _ := discoveryInvoker(t, map[string]func() (*pluginsdk.PluginActionResponse, error){
			"kandev-plugin-bitbucket": func() (*pluginsdk.PluginActionResponse, error) {
				return &pluginsdk.PluginActionResponse{Body: []byte(`{"repository":{"provider_id":"bitbucket"}}`)}, nil
			},
			"kandev-plugin-forgejo": func() (*pluginsdk.PluginActionResponse, error) {
				return &pluginsdk.PluginActionResponse{Status: 404}, nil
			},
		})
		result, err := svc.discoverRepositoryProviders(t.Context(), "workspace-1",
			RepositoryProviderDiscoveryRequest{URL: discoveryTestURL}, invoke)
		if err != nil {
			t.Fatalf("discoverRepositoryProviders: %v", err)
		}
		if result.Failure == nil || result.Failure.Code != RepositoryProviderErrorInvalid {
			t.Fatalf("failure = %v, want invalid ahead of not-found", result.Failure)
		}
	})

	t.Run("caller cancellation discards every candidate", func(t *testing.T) {
		svc, _, _ := newTestService(t)
		installRepositoryProviderPlugin(t, svc, "kandev-plugin-bitbucket", "bitbucket")
		installRepositoryProviderPlugin(t, svc, "kandev-plugin-forgejo", "forgejo")
		ctx, cancel := context.WithCancel(t.Context())
		invoke, _ := discoveryInvoker(t, map[string]func() (*pluginsdk.PluginActionResponse, error){
			"kandev-plugin-bitbucket": func() (*pluginsdk.PluginActionResponse, error) {
				return &pluginsdk.PluginActionResponse{Body: discoveryDescriptor("bitbucket", "workspace-a", "repo-1")}, nil
			},
			"kandev-plugin-forgejo": func() (*pluginsdk.PluginActionResponse, error) {
				cancel()
				return nil, context.Canceled
			},
		})
		result, err := svc.discoverRepositoryProviders(ctx, "workspace-1",
			RepositoryProviderDiscoveryRequest{URL: discoveryTestURL}, invoke)
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("error = %v, want caller cancellation", err)
		}
		if result != nil {
			t.Fatalf("result = %+v, want no candidate set after cancellation", result)
		}
	})

	t.Run("the discovery deadline is an unavailable operation error", func(t *testing.T) {
		svc, _, _ := newTestService(t)
		installRepositoryProviderPlugin(t, svc, "kandev-plugin-bitbucket", "bitbucket")
		invoke := func(ctx context.Context, _ string, _ pluginDispatchGeneration, _ *pluginsdk.PluginActionRequest) (*pluginsdk.PluginActionResponse, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		}
		result, err := svc.discoverRepositoryProvidersWithin(t.Context(), "workspace-1",
			RepositoryProviderDiscoveryRequest{URL: discoveryTestURL}, invoke, time.Millisecond)
		var providerErr *RepositoryProviderError
		if !errors.As(err, &providerErr) || providerErr.Code != RepositoryProviderErrorUnavailable {
			t.Fatalf("error = %v, want unavailable", err)
		}
		if result != nil {
			t.Fatalf("result = %+v, want no partial candidate set", result)
		}
	})

	t.Run("a replaced installation cannot authorize a candidate", func(t *testing.T) {
		svc, _, _ := newTestService(t)
		installRepositoryProviderPlugin(t, svc, "kandev-plugin-bitbucket", "bitbucket")
		invoke, _ := discoveryInvoker(t, map[string]func() (*pluginsdk.PluginActionResponse, error){
			"kandev-plugin-bitbucket": func() (*pluginsdk.PluginActionResponse, error) {
				if err := svc.Disable("kandev-plugin-bitbucket"); err != nil {
					t.Errorf("disable provider: %v", err)
				}
				return &pluginsdk.PluginActionResponse{Body: discoveryDescriptor("bitbucket", "workspace-a", "repo-1")}, nil
			},
		})
		result, err := svc.discoverRepositoryProviders(t.Context(), "workspace-1",
			RepositoryProviderDiscoveryRequest{URL: discoveryTestURL}, invoke)
		var providerErr *RepositoryProviderError
		if !errors.As(err, &providerErr) || providerErr.Code != RepositoryProviderErrorUnavailable {
			t.Fatalf("error = %v, want unavailable after eligibility changed", err)
		}
		if result != nil {
			t.Fatalf("result = %+v, want no candidate from a replaced installation", result)
		}
	})

	t.Run("discovery never exceeds four concurrent inspections", func(t *testing.T) {
		svc, _, _ := newTestService(t)
		providers := []string{"alpha", "bravo", "charlie", "delta", "echo", "foxtrot"}
		responses := make(map[string]func() (*pluginsdk.PluginActionResponse, error), len(providers))
		for index, provider := range providers {
			pluginID := fmt.Sprintf("kandev-plugin-%s", provider)
			installRepositoryProviderPlugin(t, svc, pluginID, provider)
			responses[pluginID] = func() (*pluginsdk.PluginActionResponse, error) {
				return &pluginsdk.PluginActionResponse{Body: discoveryDescriptor(provider, "scope", fmt.Sprintf("repo-%d", index))}, nil
			}
		}
		var inFlight, peak atomic.Int32
		release := make(chan struct{})
		reached := make(chan struct{}, len(providers))
		base, _ := discoveryInvoker(t, responses)
		invoke := func(ctx context.Context, id string, generation pluginDispatchGeneration, action *pluginsdk.PluginActionRequest) (*pluginsdk.PluginActionResponse, error) {
			current := inFlight.Add(1)
			defer inFlight.Add(-1)
			for {
				observed := peak.Load()
				if current <= observed || peak.CompareAndSwap(observed, current) {
					break
				}
			}
			reached <- struct{}{}
			select {
			case <-release:
			case <-ctx.Done():
				return nil, ctx.Err()
			}
			return base(ctx, id, generation, action)
		}

		type discoveryResult struct {
			result *RepositoryProviderDiscoveryResult
			err    error
		}
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		done := make(chan discoveryResult, 1)
		go func() {
			result, err := svc.discoverRepositoryProviders(ctx, "workspace-1",
				RepositoryProviderDiscoveryRequest{URL: discoveryTestURL}, invoke)
			done <- discoveryResult{result: result, err: err}
		}()

		// Wait for the bound to fill before releasing any inspection, so a
		// serialized implementation cannot pass on a peak of one.
		filled := time.NewTimer(5 * time.Second)
		defer filled.Stop()
		for range maxRepositoryDiscoveryConcurrency {
			select {
			case <-reached:
			case <-filled.C:
				cancel()
				close(release)
				<-done
				t.Fatal("fewer than four inspections ran concurrently")
			}
		}
		if got := inFlight.Load(); got != maxRepositoryDiscoveryConcurrency {
			t.Fatalf("in-flight inspections = %d, want %d", got, maxRepositoryDiscoveryConcurrency)
		}
		if got := peak.Load(); got != maxRepositoryDiscoveryConcurrency {
			t.Fatalf("peak concurrency = %d, want %d", got, maxRepositoryDiscoveryConcurrency)
		}

		close(release)
		select {
		case outcome := <-done:
			if outcome.err != nil {
				t.Fatalf("discoverRepositoryProviders: %v", outcome.err)
			}
			if len(outcome.result.Candidates) != len(providers) {
				t.Fatalf("candidates = %d, want %d", len(outcome.result.Candidates), len(providers))
			}
		case <-time.After(10 * time.Second):
			t.Fatal("discovery did not finish after releasing the inspections")
		}
		if peak.Load() > maxRepositoryDiscoveryConcurrency {
			t.Fatalf("peak concurrency = %d, want at most %d", peak.Load(), maxRepositoryDiscoveryConcurrency)
		}
	})

	t.Run("logs one sanitized discovery outcome", func(t *testing.T) {
		svc, _, _ := newTestService(t)
		installRepositoryProviderPlugin(t, svc, "kandev-plugin-bitbucket", "bitbucket")
		core, observed := observer.New(zapcore.InfoLevel)
		log, err := logger.NewFromZap(zap.New(core))
		if err != nil {
			t.Fatalf("create observed logger: %v", err)
		}
		svc.log = log
		invoke, _ := discoveryInvoker(t, map[string]func() (*pluginsdk.PluginActionResponse, error){
			"kandev-plugin-bitbucket": func() (*pluginsdk.PluginActionResponse, error) {
				return &pluginsdk.PluginActionResponse{Body: discoveryDescriptor("bitbucket", "workspace-a", "repo-1")}, nil
			},
		})
		if _, err := svc.discoverRepositoryProviders(t.Context(), "workspace-1",
			RepositoryProviderDiscoveryRequest{URL: discoveryTestURL}, invoke); err != nil {
			t.Fatalf("discoverRepositoryProviders: %v", err)
		}
		entries := observed.FilterMessage(repositoryDiscoveryLogEvent).All()
		if len(entries) != 1 {
			t.Fatalf("discovery log entries = %d, want one", len(entries))
		}
		fields := entries[0].ContextMap()
		if fmt.Sprint(fields["candidate_count"]) != "1" || fmt.Sprint(fields["match_count"]) != "1" ||
			fmt.Sprint(fields["failure_category"]) != repositoryProviderInspectionNone {
			t.Fatalf("discovery log fields = %v", fields)
		}
		if strings.Contains(fmt.Sprint(fields), discoveryTestURL) {
			t.Fatalf("discovery log leaked the repository URL: %v", fields)
		}
	})

	t.Run("no eligible provider returns an empty result", func(t *testing.T) {
		svc, _, _ := newTestService(t)
		installRepositoryProviderPlugin(t, svc, "kandev-plugin-bitbucket", "bitbucket")
		installRepositoryProviderPlugin(t, svc, "kandev-plugin-forgejo", "forgejo")
		if err := svc.Disable("kandev-plugin-forgejo"); err != nil {
			t.Fatalf("disable forgejo: %v", err)
		}
		invoke, calls := discoveryInvoker(t, map[string]func() (*pluginsdk.PluginActionResponse, error){
			"kandev-plugin-bitbucket": func() (*pluginsdk.PluginActionResponse, error) {
				return &pluginsdk.PluginActionResponse{Body: []byte(`{"matched":false}`)}, nil
			},
		})
		result, err := svc.discoverRepositoryProviders(t.Context(), "workspace-1",
			RepositoryProviderDiscoveryRequest{URL: discoveryTestURL}, invoke)
		if err != nil {
			t.Fatalf("discoverRepositoryProviders: %v", err)
		}
		if len(*calls) != 1 || len(result.Candidates) != 0 || result.Failure != nil {
			t.Fatalf("calls=%d result=%+v, want only the active provider inspected", len(*calls), result)
		}
	})
}

func installRepositoryProviderPlugin(t *testing.T, svc *Service, id, provider string) {
	t.Helper()
	if _, err := svc.Install(t.Context(), repositoryProviderInspectPackage(t, id, provider)); err != nil {
		t.Fatalf("install %s: %v", id, err)
	}
}

// repositoryProviderInspectPackage builds a package whose manifest declares one
// repository provider and the workspace inspection capability.
func repositoryProviderInspectPackage(t *testing.T, id, provider string) *bytes.Buffer {
	t.Helper()
	manifestYAML := fmt.Sprintf(`
id: %s
api_version: 1
version: "1.0.0"
display_name: Test VCS
repository_providers: [%s]
actions:
  - key: repositories.inspect
    scope: workspace
    max_body_bytes: 16384
runtime:
  type: binary
  executables:
    %s-%s: server/plugin
`, id, provider, runtime.GOOS, runtime.GOARCH)
	var buffer bytes.Buffer
	if err := pkgtartest.WritePackage(&buffer, map[string][]byte{
		"manifest.yaml": []byte(manifestYAML),
		"server/plugin": []byte("#!/bin/sh\necho fake\n"),
	}); err != nil {
		t.Fatalf("WritePackage: %v", err)
	}
	return &buffer
}

func TestDiscoverRepositoryProvidersLogsEveryExit(t *testing.T) {
	tests := []struct {
		name         string
		request      RepositoryProviderDiscoveryRequest
		run          func(*Service, context.Context) error
		wantCategory string
		wantMatches  string
	}{
		{
			name:    "success",
			request: RepositoryProviderDiscoveryRequest{URL: discoveryTestURL},
			run: func(svc *Service, ctx context.Context) error {
				invoke := func(context.Context, string, pluginDispatchGeneration, *pluginsdk.PluginActionRequest) (*pluginsdk.PluginActionResponse, error) {
					return &pluginsdk.PluginActionResponse{Body: discoveryDescriptor("bitbucket", "workspace-a", "repo-1")}, nil
				}
				_, err := svc.discoverRepositoryProviders(ctx, "workspace-1",
					RepositoryProviderDiscoveryRequest{URL: discoveryTestURL}, invoke)
				return err
			},
			wantCategory: repositoryProviderInspectionNone,
			wantMatches:  "1",
		},
		{
			name:    "invalid request",
			request: RepositoryProviderDiscoveryRequest{},
			run: func(svc *Service, ctx context.Context) error {
				_, err := svc.discoverRepositoryProviders(ctx, "workspace-1", RepositoryProviderDiscoveryRequest{}, nil)
				return err
			},
			wantCategory: "invalid",
			wantMatches:  "0",
		},
		{
			name:    "caller cancellation",
			request: RepositoryProviderDiscoveryRequest{URL: discoveryTestURL},
			run: func(svc *Service, ctx context.Context) error {
				canceled, cancel := context.WithCancel(ctx)
				cancel()
				_, err := svc.discoverRepositoryProviders(canceled, "workspace-1",
					RepositoryProviderDiscoveryRequest{URL: discoveryTestURL}, nil)
				return err
			},
			wantCategory: repositoryDiscoveryCanceled,
			wantMatches:  "0",
		},
		{
			name:    "discovery deadline",
			request: RepositoryProviderDiscoveryRequest{URL: discoveryTestURL},
			run: func(svc *Service, ctx context.Context) error {
				invoke := func(ctx context.Context, _ string, _ pluginDispatchGeneration, _ *pluginsdk.PluginActionRequest) (*pluginsdk.PluginActionResponse, error) {
					<-ctx.Done()
					return nil, ctx.Err()
				}
				_, err := svc.discoverRepositoryProvidersWithin(ctx, "workspace-1",
					RepositoryProviderDiscoveryRequest{URL: discoveryTestURL}, invoke, time.Millisecond)
				return err
			},
			wantCategory: repositoryDiscoveryDeadlineExceeded,
			wantMatches:  "0",
		},
		{
			name:    "changed eligibility",
			request: RepositoryProviderDiscoveryRequest{URL: discoveryTestURL},
			run: func(svc *Service, ctx context.Context) error {
				invoke := func(context.Context, string, pluginDispatchGeneration, *pluginsdk.PluginActionRequest) (*pluginsdk.PluginActionResponse, error) {
					if err := svc.Disable("kandev-plugin-bitbucket"); err != nil {
						t.Errorf("disable provider: %v", err)
					}
					return &pluginsdk.PluginActionResponse{Body: discoveryDescriptor("bitbucket", "workspace-a", "repo-1")}, nil
				}
				_, err := svc.discoverRepositoryProviders(ctx, "workspace-1",
					RepositoryProviderDiscoveryRequest{URL: discoveryTestURL}, invoke)
				return err
			},
			wantCategory: repositoryDiscoveryEligibility,
			wantMatches:  "0",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			svc, _, _ := newTestService(t)
			installRepositoryProviderPlugin(t, svc, "kandev-plugin-bitbucket", "bitbucket")
			core, observed := observer.New(zapcore.InfoLevel)
			log, err := logger.NewFromZap(zap.New(core))
			if err != nil {
				t.Fatalf("create observed logger: %v", err)
			}
			svc.log = log
			_ = test.run(svc, t.Context())

			entries := observed.FilterMessage(repositoryDiscoveryLogEvent).All()
			if len(entries) != 1 {
				t.Fatalf("discovery completion logs = %d, want exactly one", len(entries))
			}
			fields := entries[0].ContextMap()
			if got := fmt.Sprint(fields["failure_category"]); got != test.wantCategory {
				t.Fatalf("failure_category = %q, want %q", got, test.wantCategory)
			}
			if got := fmt.Sprint(fields["match_count"]); got != test.wantMatches {
				t.Fatalf("match_count = %q, want %q", got, test.wantMatches)
			}
			if strings.Contains(fmt.Sprint(fields), discoveryTestURL) {
				t.Fatalf("discovery log leaked the repository URL: %v", fields)
			}
		})
	}
}

func TestDiscoverRepositoryProvidersHonorsCancellationDuringDispatchLease(t *testing.T) {
	tests := []struct {
		name      string
		run       func(*Service) error
		wantError func(error) bool
	}{
		{
			name: "caller cancellation",
			run: func(svc *Service) error {
				ctx, cancel := context.WithCancel(t.Context())
				cancel()
				_, err := svc.DiscoverRepositoryProviders(ctx, "workspace-1",
					RepositoryProviderDiscoveryRequest{URL: discoveryTestURL})
				return err
			},
			wantError: func(err error) bool { return errors.Is(err, context.Canceled) },
		},
		{
			name: "caller deadline",
			run: func(svc *Service) error {
				ctx, cancel := context.WithTimeout(t.Context(), 150*time.Millisecond)
				defer cancel()
				_, err := svc.DiscoverRepositoryProviders(ctx, "workspace-1",
					RepositoryProviderDiscoveryRequest{URL: discoveryTestURL})
				return err
			},
			wantError: func(err error) bool { return errors.Is(err, context.DeadlineExceeded) },
		},
		{
			name: "discovery deadline",
			run: func(svc *Service) error {
				_, err := svc.discoverRepositoryProvidersWithin(t.Context(), "workspace-1",
					RepositoryProviderDiscoveryRequest{URL: discoveryTestURL}, svc.InvokeAction, 150*time.Millisecond)
				return err
			},
			wantError: func(err error) bool {
				var providerErr *RepositoryProviderError
				return errors.As(err, &providerErr) && providerErr.Code == RepositoryProviderErrorUnavailable
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			svc, _, _ := newTestService(t)
			installRepositoryProviderPlugin(t, svc, "kandev-plugin-bitbucket", "bitbucket")
			// Hold the lifecycle write lease the way UpdateConfig does for the
			// duration of a config commit and restart.
			lock := svc.dispatchLocks.lockFor("kandev-plugin-bitbucket")
			lock.Lock()
			released := false
			defer func() {
				if !released {
					lock.Unlock()
				}
			}()

			done := make(chan error, 1)
			go func() { done <- test.run(svc) }()
			select {
			case err := <-done:
				if !test.wantError(err) {
					t.Fatalf("error = %v, want a cancellation-class failure", err)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("discovery did not return while the dispatch write lease was held")
			}
			lock.Unlock()
			released = true
		})
	}
}

func TestDiscoverRepositoryProvidersRejectsCanceledEmptySnapshot(t *testing.T) {
	svc, _, _ := newTestService(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	result, err := svc.DiscoverRepositoryProviders(ctx, "workspace-1",
		RepositoryProviderDiscoveryRequest{URL: discoveryTestURL})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want caller cancellation", err)
	}
	if result != nil {
		t.Fatalf("result = %+v, want none", result)
	}
}
