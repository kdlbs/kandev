package launcher

import (
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/common/config"
)

func TestStartupFailureFormattingIncludesSafeEvidenceAndRecovery(t *testing.T) {
	const healthToken = "do-not-print-this-token"
	endpoints := backendEndpointSet{
		bindHosts: []string{"192.0.2.10"},
		healthTargets: []string{
			"http://192.0.2.10:38429/health",
		},
		accessURL: "http://192.0.2.10:38429",
	}
	err := &backendHealthError{
		Class:       healthFailureForeign,
		EndpointSet: endpoints,
		Observations: []healthObservation{{
			URL:        endpoints.healthTargets[0],
			Outcome:    healthOutcomeForeignProcess,
			SafeDetail: "missing or mismatched launcher token",
		}},
	}
	cfg := &config.Config{
		Server: config.ServerConfig{Host: "192.0.2.10"},
		Source: config.ConfigSource{FilePath: "/etc/kandev/config.yaml"},
	}

	got := formatStartupFailure(err, endpoints, cfg, "/srv/kandev/logs/backend-logs.log", true)
	for _, want := range []string{
		"startup failed",
		"foreign process",
		"effective bind addresses: 192.0.2.10",
		"http://192.0.2.10:38429/health: foreign_process",
		"configuration source: /etc/kandev/config.yaml",
		"backend log: /srv/kandev/logs/backend-logs.log",
		"launcher stopped the backend after readiness failed",
		"free the selected port or choose another backend port",
		startupTroubleshootingURL,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("formatted failure missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, healthToken) || strings.Contains(got, "X-Kandev-Desktop-Health-Token") {
		t.Fatalf("formatted failure exposed health-token material:\n%s", got)
	}
}
