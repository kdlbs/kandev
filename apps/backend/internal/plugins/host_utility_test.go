package plugins

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/plugins/manifest"
	"github.com/kandev/kandev/pkg/pluginsdk"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type fakeDefaultUtilityProfileSource struct {
	profileID string
	err       error
	calls     int
}

func (f *fakeDefaultUtilityProfileSource) GetDefaultUtilityAgentProfileID(context.Context) (string, error) {
	f.calls++
	return f.profileID, f.err
}

type fakeConfigReader struct {
	configs map[string]any
	err     error
	calls   int
}

func (f *fakeConfigReader) GetConfig(string) (map[string]any, error) {
	f.calls++
	return f.configs, f.err
}

type fakeUtilityRunner struct {
	calls        int
	gotProfileID string
	gotPrompt    string
	text         string
	err          error
}

func (f *fakeUtilityRunner) ExecuteProfilePrompt(_ context.Context, profileID, prompt string) (string, error) {
	f.calls++
	f.gotProfileID, f.gotPrompt = profileID, prompt
	return f.text, f.err
}

func configuredUtilityHost(t *testing.T) *testDataHost {
	t.Helper()
	d := newTestDataHost(manifest.Capabilities{AgentInvoke: true})
	d.host.configs = &fakeConfigReader{configs: map[string]any{
		"utility_agent": "profile-from-plugin-config",
		"agent_profile": "profile-from-plugin-config",
	}}
	d.defaultProfile.profileID = "profile-default"
	d.profiles.profilesByID = map[string]*AgentProfile{
		"profile-default":  {Enabled: true, InferenceCapable: true},
		"profile-override": {Enabled: true, InferenceCapable: true},
	}
	d.utilRun.text = "the summary"
	return d
}

func TestPluginHost_InvokeUtilityAgent_DeniedWithoutCapability(t *testing.T) {
	d := newTestDataHost(manifest.Capabilities{})
	_, err := d.host.InvokeUtilityAgent(context.Background(), "hi")
	assertPermissionDenied(t, err, "agent_invoke")
	if d.defaultProfile.calls != 0 || d.profiles.profileCalls != 0 || d.utilRun.calls != 0 {
		t.Fatalf("unauthorized call touched default %d times, profiles %d times, runner %d times", d.defaultProfile.calls, d.profiles.profileCalls, d.utilRun.calls)
	}
}

func TestPluginHost_InvokeUtilityAgent_DefaultAndOverride(t *testing.T) {
	t.Run("zero options uses current default", func(t *testing.T) {
		d := configuredUtilityHost(t)

		got, err := d.host.InvokeUtilityAgent(context.Background(), "summarize yesterday")
		if err != nil || got != "the summary" {
			t.Fatalf("InvokeUtilityAgent() = (%q, %v)", got, err)
		}
		if d.defaultProfile.calls != 1 || d.utilRun.gotProfileID != "profile-default" {
			t.Fatalf("default calls = %d, runner profile = %q", d.defaultProfile.calls, d.utilRun.gotProfileID)
		}
	})

	t.Run("one empty option uses current default", func(t *testing.T) {
		d := configuredUtilityHost(t)

		got, err := d.host.InvokeUtilityAgent(context.Background(), "summarize yesterday", pluginsdk.UtilityAgentOptions{})
		if err != nil || got != "the summary" {
			t.Fatalf("InvokeUtilityAgent() = (%q, %v)", got, err)
		}
		if d.defaultProfile.calls != 1 || d.utilRun.gotProfileID != "profile-default" {
			t.Fatalf("default calls = %d, runner profile = %q", d.defaultProfile.calls, d.utilRun.gotProfileID)
		}
	})

	t.Run("non-empty option uses exact override", func(t *testing.T) {
		d := configuredUtilityHost(t)

		got, err := d.host.InvokeUtilityAgent(context.Background(), "summarize yesterday", pluginsdk.UtilityAgentOptions{ProfileID: "profile-override"})
		if err != nil || got != "the summary" {
			t.Fatalf("InvokeUtilityAgent() = (%q, %v)", got, err)
		}
		if d.defaultProfile.calls != 0 {
			t.Fatalf("explicit override read the default %d times", d.defaultProfile.calls)
		}
		if d.utilRun.gotProfileID != "profile-override" {
			t.Fatalf("runner profile = %q, want profile-override", d.utilRun.gotProfileID)
		}
	})
}

func TestPluginHost_InvokeUtilityAgent_IgnoresPluginConfig(t *testing.T) {
	d := configuredUtilityHost(t)
	d.host.configs = &fakeConfigReader{err: errors.New("plugin config must not be read")}
	d.host.configSchema = map[string]any{"properties": map[string]any{
		"utility_agent": map[string]any{"type": "string", "format": "utility-agent"},
	}}

	got, err := d.host.InvokeUtilityAgent(context.Background(), "summarize")
	if err != nil || got != "the summary" {
		t.Fatalf("InvokeUtilityAgent() = (%q, %v)", got, err)
	}
	if d.host.configs.(*fakeConfigReader).calls != 0 {
		t.Fatalf("InvokeUtilityAgent() read plugin config %d times", d.host.configs.(*fakeConfigReader).calls)
	}
	if d.utilRun.gotProfileID != "profile-default" {
		t.Fatalf("runner profile = %q, want profile-default", d.utilRun.gotProfileID)
	}
}

func TestPluginHost_InvokeUtilityAgent_DefaultChangesBetweenCalls(t *testing.T) {
	d := configuredUtilityHost(t)

	if _, err := d.host.InvokeUtilityAgent(context.Background(), "first"); err != nil {
		t.Fatalf("first InvokeUtilityAgent() error = %v", err)
	}
	d.defaultProfile.profileID = "profile-override"
	if _, err := d.host.InvokeUtilityAgent(context.Background(), "second"); err != nil {
		t.Fatalf("second InvokeUtilityAgent() error = %v", err)
	}
	if d.defaultProfile.calls != 2 {
		t.Fatalf("default calls = %d, want 2", d.defaultProfile.calls)
	}
	if d.utilRun.gotProfileID != "profile-override" {
		t.Fatalf("second runner profile = %q, want profile-override", d.utilRun.gotProfileID)
	}
}

func TestPluginHost_InvokeUtilityAgent_ExplicitOverrideDoesNotReadDefault(t *testing.T) {
	d := configuredUtilityHost(t)
	d.defaultProfile.err = status.Error(codes.Unavailable, "default store unavailable")

	got, err := d.host.InvokeUtilityAgent(context.Background(), "explicit", pluginsdk.UtilityAgentOptions{ProfileID: "profile-override"})
	if err != nil || got != "the summary" {
		t.Fatalf("InvokeUtilityAgent() = (%q, %v)", got, err)
	}
	if d.defaultProfile.calls != 0 {
		t.Fatalf("explicit override read the default %d times", d.defaultProfile.calls)
	}
}

func TestPluginHost_InvokeUtilityAgent_RejectsMultipleOptions(t *testing.T) {
	d := configuredUtilityHost(t)

	_, err := d.host.InvokeUtilityAgent(
		context.Background(),
		"summarize",
		pluginsdk.UtilityAgentOptions{ProfileID: "profile-default"},
		pluginsdk.UtilityAgentOptions{ProfileID: "profile-override"},
	)
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("InvokeUtilityAgent() status = %s, want %s", status.Code(err), codes.InvalidArgument)
	}
	if d.defaultProfile.calls != 0 || d.profiles.profileCalls != 0 || d.utilRun.calls != 0 {
		t.Fatalf("invalid options touched default %d times, profiles %d times, runner %d times", d.defaultProfile.calls, d.profiles.profileCalls, d.utilRun.calls)
	}
}

func TestPluginHost_InvokeUtilityAgent_SelectionErrors(t *testing.T) {
	tests := []struct {
		name             string
		defaultProfile   string
		options          []pluginsdk.UtilityAgentOptions
		profiles         map[string]*AgentProfile
		want             string
		wantDefaultCalls int
	}{
		{
			name:             "empty default",
			defaultProfile:   "",
			want:             "no default agent profile configured",
			wantDefaultCalls: 1,
		},
		{
			name:             "missing default",
			defaultProfile:   "missing",
			profiles:         map[string]*AgentProfile{},
			want:             `agent profile "missing" not found`,
			wantDefaultCalls: 1,
		},
		{
			name:             "ineligible default",
			defaultProfile:   "disabled",
			profiles:         map[string]*AgentProfile{"disabled": {Enabled: false, InferenceCapable: true}},
			want:             `agent profile "disabled" is not eligible for utility execution`,
			wantDefaultCalls: 1,
		},
		{
			name:           "missing explicit override",
			defaultProfile: "profile-default",
			options:        []pluginsdk.UtilityAgentOptions{{ProfileID: "missing"}},
			profiles:       map[string]*AgentProfile{},
			want:           `agent profile "missing" not found`,
		},
		{
			name:           "ineligible explicit override",
			defaultProfile: "profile-default",
			options:        []pluginsdk.UtilityAgentOptions{{ProfileID: "disabled"}},
			profiles:       map[string]*AgentProfile{"disabled": {Enabled: false, InferenceCapable: true}},
			want:           `agent profile "disabled" is not eligible for utility execution`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := configuredUtilityHost(t)
			d.defaultProfile.profileID = tt.defaultProfile
			if tt.profiles != nil {
				d.profiles.profilesByID = tt.profiles
			}

			_, err := d.host.InvokeUtilityAgent(context.Background(), "summarize", tt.options...)
			if status.Code(err) != codes.FailedPrecondition || status.Convert(err).Message() != tt.want {
				t.Fatalf("InvokeUtilityAgent() error = %v, want FailedPrecondition %q", err, tt.want)
			}
			if d.defaultProfile.calls != tt.wantDefaultCalls || d.utilRun.calls != 0 {
				t.Fatalf("default calls = %d, want %d; runner calls = %d", d.defaultProfile.calls, tt.wantDefaultCalls, d.utilRun.calls)
			}
		})
	}
}

func TestPluginHost_InvokeUtilityAgent_PreservesDefaultSourceFailures(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		code codes.Code
	}{
		{name: "storage", err: status.Error(codes.Unavailable, "settings unavailable"), code: codes.Unavailable},
		{name: "cancellation", err: context.Canceled, code: codes.Canceled},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := configuredUtilityHost(t)
			d.defaultProfile.err = tc.err

			_, err := d.host.InvokeUtilityAgent(context.Background(), "summarize")
			if status.Code(err) != tc.code {
				t.Fatalf("InvokeUtilityAgent() status = %s, want %s; err = %v", status.Code(err), tc.code, err)
			}
			if d.profiles.profileCalls != 0 || d.utilRun.calls != 0 {
				t.Fatalf("source failure touched profiles %d times and runner %d times", d.profiles.profileCalls, d.utilRun.calls)
			}
		})
	}
}

func TestPluginHost_InvokeUtilityAgent_PreservesRunnerFailure(t *testing.T) {
	runnerErr := status.Error(codes.Unavailable, "agentctl unavailable")
	d := configuredUtilityHost(t)
	d.utilRun.err = runnerErr

	_, err := d.host.InvokeUtilityAgent(context.Background(), "summarize")
	if !errors.Is(err, runnerErr) || status.Code(err) != codes.Unavailable {
		t.Fatalf("InvokeUtilityAgent() error = %v, want runner error %v", err, runnerErr)
	}
}

func TestPluginHost_InvokeUtilityAgent_MapsRunnerProfileErrors(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		want string
	}{
		{name: "deleted", err: ErrAgentProfileNotFound, want: `agent profile "profile-default" not found`},
		{name: "ineligible", err: ErrAgentProfileIneligible, want: `agent profile "profile-default" is not eligible for utility execution`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := configuredUtilityHost(t)
			d.utilRun.err = tc.err

			_, err := d.host.InvokeUtilityAgent(context.Background(), "summarize")
			if status.Code(err) != codes.FailedPrecondition || status.Convert(err).Message() != tc.want {
				t.Fatalf("InvokeUtilityAgent() error = %v, want FailedPrecondition %q", err, tc.want)
			}
		})
	}
}
