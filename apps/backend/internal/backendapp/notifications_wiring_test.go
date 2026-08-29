package backendapp

import (
	"reflect"
	"testing"

	"github.com/kandev/kandev/internal/auth"
	"github.com/kandev/kandev/internal/common/config"
)

// TestNotificationAuthEnforcedIsDisabledWithoutAnAuthService pins the
// single-user case: no auth service means one account, so an unresolvable
// notification owner may still fall back to the default user.
func TestNotificationAuthEnforcedIsDisabledWithoutAnAuthService(t *testing.T) {
	if notificationAuthEnforced(nil)() {
		t.Fatal("a nil auth service must report authentication as not enforced")
	}
}

// TestNotificationAuthEnforcedFollowsAnEnabledAuthService is the guard that
// matters: while authentication is enforced, an unresolvable owner must make
// the notification service drop the notification rather than redirect another
// user's task title to the default administrator's webhook.
func TestNotificationAuthEnforcedFollowsAnEnabledAuthService(t *testing.T) {
	cfg := &config.Config{}
	cfg.Features.Auth = true
	cfg.Auth.SessionTTLHours = 720
	svc := newEnabledAuthService(t, cfg)

	if svc.Mode() != auth.ModeEnabled {
		t.Fatalf("mode = %s, want enabled", svc.Mode())
	}
	if !notificationAuthEnforced(svc)() {
		t.Fatal("an enabled auth service must report authentication as enforced")
	}
}

// TestNotificationAuthEnforcedFollowsADisabledAuthService keeps single-user
// installs byte-identical: with the feature off, notifications still fall back
// to the pre-auth default user.
func TestNotificationAuthEnforcedFollowsADisabledAuthService(t *testing.T) {
	svc := newDisabledAuthService(t)

	if svc.Mode() != auth.ModeDisabled {
		t.Fatalf("mode = %s, want disabled", svc.Mode())
	}
	if notificationAuthEnforced(svc)() {
		t.Fatal("a disabled auth service must report authentication as not enforced")
	}
}

// TestProvideGatewayTakesTheAuthService fails if the auth service is dropped
// from the signature, which would leave the notification service without any
// way to know that more than one account exists.
func TestProvideGatewayTakesTheAuthService(t *testing.T) {
	signature := reflect.TypeOf(provideGateway)
	want := reflect.TypeOf((*auth.Service)(nil))
	for index := range signature.NumIn() {
		if signature.In(index) == want {
			return
		}
	}
	t.Fatal("provideGateway does not accept an *auth.Service")
}
