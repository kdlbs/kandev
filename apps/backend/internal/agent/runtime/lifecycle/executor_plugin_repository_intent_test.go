package lifecycle

import (
	"errors"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/githubauth"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/pkg/pluginsdk"
)

func intPtr(value int) *int { return &value }

func TestRepositoryIntentOrdersByPositionThenIDWithMissingPositionLast(t *testing.T) {
	intent, err := buildPluginRepositoryIntent([]PluginRepositorySource{
		{RepositoryID: "b", Locator: "https://example.test/b.git", Position: intPtr(1)},
		{RepositoryID: "z", Locator: "https://example.test/z.git"},
		{RepositoryID: "a", Locator: "https://example.test/a.git"},
		{RepositoryID: "c", Locator: "https://example.test/c.git", Position: intPtr(1)},
		{RepositoryID: "d", Locator: "https://example.test/d.git", Position: intPtr(0)},
	})
	if err != nil {
		t.Fatalf("buildPluginRepositoryIntent() error = %v", err)
	}
	var got []string
	for _, entry := range intent {
		got = append(got, entry.GetId())
	}
	if want := "d,b,c,a,z"; strings.Join(got, ",") != want {
		t.Fatalf("order = %v, want %s", got, want)
	}
}

func TestRepositoryIntentIDTieBreakIsByteOrder(t *testing.T) {
	intent, err := buildPluginRepositoryIntent([]PluginRepositorySource{
		{RepositoryID: "a", Locator: "https://example.test/a.git", Position: intPtr(0)},
		{RepositoryID: "B", Locator: "https://example.test/b.git", Position: intPtr(0)},
	})
	if err != nil {
		t.Fatal(err)
	}
	if intent[0].GetId() != "B" || intent[1].GetId() != "a" {
		t.Fatalf("order = %s,%s, want byte order B,a", intent[0].GetId(), intent[1].GetId())
	}
}

func TestRepositoryIntentSendsUnsetFieldsAsEmptyStringsAndIsDeterministic(t *testing.T) {
	sources := []PluginRepositorySource{{RepositoryID: "r1", Locator: "https://example.test/r1.git"}}
	first, err := buildPluginRepositoryIntent(sources)
	if err != nil {
		t.Fatal(err)
	}
	if entry := first[0]; entry.GetBranch() != "" || entry.GetBaseBranch() != "" || entry.GetCheckoutRef() != "" {
		t.Fatalf("unset fields = %q %q %q, want empty strings", entry.GetBranch(), entry.GetBaseBranch(), entry.GetCheckoutRef())
	}
	second, _ := buildPluginRepositoryIntent(sources)
	a, _ := pluginExecutorInputDigestWith(models.ExecutorProviderLaunchProfile{}, first, nil)
	b, _ := pluginExecutorInputDigestWith(models.ExecutorProviderLaunchProfile{}, second, nil)
	if a != b {
		t.Fatalf("digest is not deterministic: %s != %s", a, b)
	}
}

func TestRepositoryIntentEmptyListIsValid(t *testing.T) {
	intent, err := buildPluginRepositoryIntent(nil)
	if err != nil || len(intent) != 0 {
		t.Fatalf("buildPluginRepositoryIntent(nil) = %v, %v, want an empty list", intent, err)
	}
}

func TestRepositoryIntentLimitsFailAsInvalidConfig(t *testing.T) {
	many := make([]PluginRepositorySource, 33)
	for i := range many {
		many[i] = PluginRepositorySource{RepositoryID: string(rune('A' + i)), Locator: "https://example.test/r.git"}
	}
	long := []PluginRepositorySource{{RepositoryID: "r", Locator: "https://example.test/" + strings.Repeat("a", 2048)}}
	for name, sources := range map[string][]PluginRepositorySource{"33 entries": many, "locator over 2048 bytes": long} {
		if _, err := buildPluginRepositoryIntent(sources); !errors.Is(err, models.ErrExecutorInvalidConfig) {
			t.Fatalf("%s: error = %v, want invalid_config", name, err)
		}
	}
	atLimit := many[:32]
	if _, err := buildPluginRepositoryIntent(atLimit); err != nil {
		t.Fatalf("32 entries: error = %v, want accepted", err)
	}
}

func TestRepositoryIntentLocatorCarriesNoCredential(t *testing.T) {
	intent, err := buildPluginRepositoryIntent([]PluginRepositorySource{
		{RepositoryID: "r1", Locator: userinfoURL("https://example.test/org/r1.git", "bot", "s3cr3t-value")},
		{RepositoryID: "r2", Locator: "https://example.test/org/r2.git?token=abc123#frag"},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range intent {
		locator := entry.GetLocator()
		if strings.Contains(locator, "s3cr3t-value") || strings.Contains(locator, "bot@") || strings.Contains(locator, "abc123") || strings.Contains(locator, "?") || strings.Contains(locator, "#") {
			t.Fatalf("locator %q still carries credential material", locator)
		}
	}
	if got := intent[0].GetLocator(); got != "https://example.test/org/r1.git" {
		t.Fatalf("locator = %q, want the bare URL", got)
	}
}

func TestAgentEnvKeepsOnlyPlainProfileEnvAndTheHostAllowlist(t *testing.T) {
	instance := map[string]string{
		"KANDEV_TASK_ID": "t1", "KANDEV_SESSION_ID": "s1", "KANDEV_INSTANCE_ID": "i1",
		"KANDEV_TASK_ENVIRONMENT_ID": "e1", "KANDEV_AGENT_PROFILE_ID": "ap", "KANDEV_EXECUTION_PROFILE_ID": "xp",
		"LANG": "C.UTF-8", "LC_ALL": "C.UTF-8", "TZ": "UTC",
		"PATH": "/usr/bin", "HOME": "/root", "EDITOR": "vi",
	}
	env, err := buildPluginAgentEnv(instance, map[string]string{"PROJECT_MODE": "ci"})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"KANDEV_TASK_ID": "t1", "KANDEV_SESSION_ID": "s1", "KANDEV_INSTANCE_ID": "i1",
		"KANDEV_TASK_ENVIRONMENT_ID": "e1", "KANDEV_AGENT_PROFILE_ID": "ap", "KANDEV_EXECUTION_PROFILE_ID": "xp",
		"LANG": "C.UTF-8", "LC_ALL": "C.UTF-8", "TZ": "UTC", "PROJECT_MODE": "ci",
	}
	if len(env) != len(want) {
		t.Fatalf("agent_env = %v, want %v", env, want)
	}
	for key, value := range want {
		if env[key] != value {
			t.Fatalf("agent_env[%s] = %q, want %q", key, env[key], value)
		}
	}
}

func TestAgentEnvNeverCarriesAnySecretClass(t *testing.T) {
	seeded := map[string]string{
		"GITHUB_TOKEN": "seed-1", "GH_TOKEN": "seed-2", "API_KEY": "seed-3", "client_secret": "seed-4",
		"DB_PASSWORD": "seed-5", "AWS_CREDENTIAL_FILE": "seed-6", "ssh_private_key": "seed-7",
		githubauth.CredentialLeaseEnv: "seed-8", githubauth.CredentialBrokerURLEnv: "seed-9",
		githubauth.CredentialHelperPathEnv: "seed-10", "GIT_TERMINAL_PROMPT": "0",
		"GIT_CONFIG_COUNT": "1", "GIT_CONFIG_KEY_0": "credential.helper", "GIT_CONFIG_VALUE_0": "seed-11",
	}
	env, err := buildPluginAgentEnv(seeded, seeded)
	if err != nil {
		t.Fatal(err)
	}
	for key, value := range env {
		if strings.HasPrefix(value, "seed-") {
			t.Fatalf("agent_env carries a seeded secret under %s", key)
		}
	}
	if len(env) != 0 {
		t.Fatalf("agent_env = %v, want empty: no allowlisted or plain safe entry was supplied", env)
	}
}

func TestAgentEnvOverLimitsFailsAndNeverTruncates(t *testing.T) {
	tooMany := map[string]string{}
	for i := 0; i < 65; i++ {
		tooMany["VAR_"+strings.Repeat("A", i%7)+string(rune('A'+i%26))+string(rune('a'+i/26))] = "v"
	}
	if len(tooMany) < 65 {
		t.Fatalf("fixture has %d entries, want 65", len(tooMany))
	}
	if _, err := buildPluginAgentEnv(nil, tooMany); !errors.Is(err, models.ErrExecutorInvalidConfig) {
		t.Fatalf("65 entries: error = %v, want invalid_config", err)
	}
	if _, err := buildPluginAgentEnv(nil, map[string]string{"BIG": strings.Repeat("x", 16*1024)}); !errors.Is(err, models.ErrExecutorInvalidConfig) {
		t.Fatalf("over 16 KiB: error = %v, want invalid_config", err)
	}
}

func TestDigestIncludesIntentAndAgentEnv(t *testing.T) {
	profile := models.ExecutorProviderLaunchProfile{ProfileID: "p"}
	base, _ := pluginExecutorInputDigestWith(profile, nil, nil)
	withIntent, _ := pluginExecutorInputDigestWith(profile, []*pluginsdk.ExecutorRepositoryIntent{{Id: "r", Branch: "one"}}, nil)
	changed, _ := pluginExecutorInputDigestWith(profile, []*pluginsdk.ExecutorRepositoryIntent{{Id: "r", Branch: "two"}}, nil)
	withEnv, _ := pluginExecutorInputDigestWith(profile, nil, map[string]string{"TZ": "UTC"})
	if base == withIntent || withIntent == changed || base == withEnv {
		t.Fatal("the digest must change with the intent and with agent_env")
	}
	plain, _ := pluginExecutorInputDigest(profile)
	if plain != base {
		t.Fatal("a launch with no intent and no env must keep the existing digest")
	}
}
