package github

import (
	"context"
	"errors"
	"net/netip"
	"reflect"
	"testing"
	"time"
)

type manifestResolverFunc func(context.Context, string, string) ([]netip.Addr, error)

func (f manifestResolverFunc) LookupNetIP(
	ctx context.Context,
	network, host string,
) ([]netip.Addr, error) {
	return f(ctx, network, host)
}

func TestDeploymentAppManifestExactPolicyAndURLs(t *testing.T) {
	resolver := manifestResolverFunc(func(_ context.Context, network, host string) ([]netip.Addr, error) {
		if network != "ip" || host != "kandev.example" {
			t.Fatalf("LookupNetIP() = %q, %q", network, host)
		}
		return []netip.Addr{netip.MustParseAddr("203.0.114.10")}, nil
	})
	baseURL, err := ValidatePublicGitHubBaseURL(
		context.Background(),
		"https://KANDEV.example:443/",
		resolver,
	)
	if err != nil {
		t.Fatalf("ValidatePublicGitHubBaseURL() error = %v", err)
	}

	submission, err := BuildDeploymentAppManifest(ManifestOwnerUser, "octocat", baseURL)
	if err != nil {
		t.Fatalf("BuildDeploymentAppManifest() error = %v", err)
	}
	if submission.Revision != DeploymentAppManifestRevision {
		t.Fatalf("Revision = %d, want %d", submission.Revision, DeploymentAppManifestRevision)
	}
	if submission.RegistrationURL != "https://github.com/settings/apps/new" {
		t.Fatalf("RegistrationURL = %q", submission.RegistrationURL)
	}
	manifest := submission.Manifest
	if manifest.Name == "" || len(manifest.Name) > 34 {
		t.Fatalf("Name = %q, want non-empty GitHub-compatible name", manifest.Name)
	}
	if manifest.URL != "https://kandev.example" ||
		manifest.RedirectURL != "https://kandev.example/api/v1/github/app/registration/callback" ||
		!reflect.DeepEqual(manifest.CallbackURLs, []string{
			"https://kandev.example/api/v1/github/personal-connection/callback",
		}) ||
		manifest.SetupURL != "https://kandev.example/api/v1/github/app/install/callback" ||
		manifest.HookAttributes.URL != "https://kandev.example/api/v1/github/app/webhook" ||
		!manifest.HookAttributes.Active {
		t.Fatalf("manifest URLs = %+v", manifest)
	}
	if !manifest.Public || !manifest.RequestOAuthOnInstall || manifest.SetupOnUpdate {
		t.Fatalf("manifest flags = public:%v oauth:%v setup_on_update:%v",
			manifest.Public, manifest.RequestOAuthOnInstall, manifest.SetupOnUpdate)
	}
	wantPermissions := map[string]string{
		"actions":        "read",
		"administration": "read",
		"checks":         "read",
		"contents":       "write",
		"issues":         "write",
		"members":        "read",
		"metadata":       "read",
		"pull_requests":  "write",
		"statuses":       "read",
		"workflows":      "write",
	}
	if !reflect.DeepEqual(manifest.DefaultPermissions, wantPermissions) {
		t.Fatalf("DefaultPermissions = %#v, want %#v", manifest.DefaultPermissions, wantPermissions)
	}
	wantEvents := []string{"installation", "installation_repositories", "github_app_authorization"}
	if !reflect.DeepEqual(manifest.DefaultEvents, wantEvents) {
		t.Fatalf("DefaultEvents = %#v, want %#v", manifest.DefaultEvents, wantEvents)
	}
}

func TestDeploymentAppManifestOrganizationOwnerURL(t *testing.T) {
	submission, err := BuildDeploymentAppManifest(
		ManifestOwnerOrganization,
		"kdlbs",
		"https://kandev.example",
	)
	if err != nil {
		t.Fatalf("BuildDeploymentAppManifest() error = %v", err)
	}
	if submission.RegistrationURL != "https://github.com/organizations/kdlbs/settings/apps/new" {
		t.Fatalf("RegistrationURL = %q", submission.RegistrationURL)
	}
}

func TestDeploymentAppManifestRejectsInvalidOwner(t *testing.T) {
	tests := []struct {
		name      string
		ownerType ManifestOwnerType
		login     string
	}{
		{name: "unknown owner", ownerType: "enterprise", login: "acme"},
		{name: "missing login", ownerType: ManifestOwnerUser},
		{name: "invalid login", ownerType: ManifestOwnerOrganization, login: "acme/widgets"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := BuildDeploymentAppManifest(tt.ownerType, tt.login, "https://kandev.example")
			if !errors.Is(err, ErrDeploymentAppManifestOwnerInvalid) {
				t.Fatalf("error = %v, want %v", err, ErrDeploymentAppManifestOwnerInvalid)
			}
		})
	}
}

func TestPublicGitHubBaseURLRejectsUnsafeOrigins(t *testing.T) {
	resolver := manifestResolverFunc(func(_ context.Context, _, _ string) ([]netip.Addr, error) {
		return []netip.Addr{netip.MustParseAddr("203.0.114.10")}, nil
	})
	tests := []string{
		"http://kandev.example",
		"https://user:pass@kandev.example",
		"https://kandev.example/path",
		"https://kandev.example?debug=true",
		"https://kandev.example#fragment",
		"https://127.0.0.1",
		"https://10.0.0.1",
		"https://169.254.1.2",
		"https://[::1]",
		"https://[fd00::1]",
		"https://[100:0:0:1::1]",
		"https://[2001::1]",
		"https://[2001:2::1]",
		"https://[2001:10::1]",
		"https://[2001:db8::1]",
		"https://[2002::1]",
		"https://[3fff::1]",
		"https://[5f00::1]",
		"https://[fec0::1]",
	}
	for _, rawURL := range tests {
		t.Run(rawURL, func(t *testing.T) {
			_, err := ValidatePublicGitHubBaseURL(context.Background(), rawURL, resolver)
			if err == nil {
				t.Fatal("error = nil, want unsafe origin rejection")
			}
		})
	}
}

func TestPublicGitHubBaseURLAllowsIANAReachableSpecialPurposeAddresses(t *testing.T) {
	for _, rawURL := range []string{
		"https://192.0.0.9",
		"https://192.0.0.10",
		"https://[2001:1::1]",
		"https://[2001:1::2]",
		"https://[2001:1::3]",
		"https://[2001:3::1]",
		"https://[2001:4:112::1]",
		"https://[2001:20::1]",
		"https://[2001:30::1]",
	} {
		t.Run(rawURL, func(t *testing.T) {
			canonical, err := ValidatePublicGitHubBaseURL(context.Background(), rawURL, nil)
			if err != nil {
				t.Fatalf("ValidatePublicGitHubBaseURL() error = %v", err)
			}
			if canonical != rawURL {
				t.Fatalf("canonical URL = %q, want %q", canonical, rawURL)
			}
		})
	}
}

func TestPublicGitHubBaseURLRejectsSiteLocalIPv6DNSResult(t *testing.T) {
	resolver := manifestResolverFunc(func(_ context.Context, _, host string) ([]netip.Addr, error) {
		if host != "site-local.example" {
			t.Fatalf("host = %q", host)
		}
		return []netip.Addr{
			netip.MustParseAddr("203.0.114.10"),
			netip.MustParseAddr("fec0::1"),
		}, nil
	})
	_, err := ValidatePublicGitHubBaseURL(context.Background(), "https://site-local.example", resolver)
	if !errors.Is(err, ErrPublicGitHubBaseURLNotGlobal) {
		t.Fatalf("error = %v, want %v", err, ErrPublicGitHubBaseURLNotGlobal)
	}
}

func TestPublicGitHubBaseURLRequiresEveryDNSResultToBeGlobal(t *testing.T) {
	resolver := manifestResolverFunc(func(_ context.Context, _, host string) ([]netip.Addr, error) {
		if host != "mixed.example" {
			t.Fatalf("host = %q", host)
		}
		return []netip.Addr{
			netip.MustParseAddr("203.0.114.10"),
			netip.MustParseAddr("192.168.1.10"),
		}, nil
	})
	_, err := ValidatePublicGitHubBaseURL(context.Background(), "https://mixed.example", resolver)
	if !errors.Is(err, ErrPublicGitHubBaseURLNotGlobal) {
		t.Fatalf("error = %v, want %v", err, ErrPublicGitHubBaseURLNotGlobal)
	}
}

func TestPublicGitHubBaseURLRejectsEmptyOrFailedDNS(t *testing.T) {
	tests := []struct {
		name     string
		resolver manifestResolverFunc
	}{
		{
			name: "empty result",
			resolver: func(context.Context, string, string) ([]netip.Addr, error) {
				return nil, nil
			},
		},
		{
			name: "lookup failure",
			resolver: func(context.Context, string, string) ([]netip.Addr, error) {
				return nil, errors.New("resolver details")
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ValidatePublicGitHubBaseURL(
				context.Background(),
				"https://unresolved.example",
				tt.resolver,
			)
			if !errors.Is(err, ErrPublicGitHubBaseURLUnresolvable) {
				t.Fatalf("error = %v, want %v", err, ErrPublicGitHubBaseURLUnresolvable)
			}
			if err != nil && errors.Is(err, context.Canceled) {
				t.Fatalf("error unexpectedly exposed resolver details: %v", err)
			}
		})
	}
}

func TestDeploymentAppManifestFlowExpiresAfterOneHourAndRejectsReplay(t *testing.T) {
	createdAt := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)
	expiresAt := DeploymentAppManifestFlowExpiresAt(createdAt)
	if got := expiresAt.Sub(createdAt); got != time.Hour {
		t.Fatalf("flow TTL = %v, want %v", got, time.Hour)
	}
	if err := ValidateDeploymentAppManifestFlow(expiresAt, nil, expiresAt.Add(-time.Nanosecond)); err != nil {
		t.Fatalf("valid flow error = %v", err)
	}
	if err := ValidateDeploymentAppManifestFlow(expiresAt, nil, expiresAt); !errors.Is(
		err,
		ErrDeploymentAppManifestStateUnavailable,
	) {
		t.Fatalf("expired flow error = %v, want unavailable", err)
	}
	consumedAt := createdAt.Add(time.Minute)
	if err := ValidateDeploymentAppManifestFlow(
		expiresAt,
		&consumedAt,
		createdAt.Add(2*time.Minute),
	); !errors.Is(err, ErrDeploymentAppManifestStateUnavailable) {
		t.Fatalf("replayed flow error = %v, want unavailable", err)
	}
}
