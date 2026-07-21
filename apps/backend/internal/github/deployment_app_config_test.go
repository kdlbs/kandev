package github

import (
	"context"
	"testing"
	"time"
)

func TestResolveAppRegistrationConfigUsesRequestedCatalogID(t *testing.T) {
	store := newTestStore(t)
	repository := NewAppRegistrationRepository(store, newFakeConnectionSecrets())
	now := time.Now().UTC()
	work := newAppRegistration("registration-work", 101, "Work", now)
	personal := newAppRegistration("registration-personal", 202, "Personal", now)
	for _, item := range []struct {
		registration *AppRegistration
		credentials  DeploymentAppCredentials
	}{
		{work, DeploymentAppCredentials{PrivateKey: "work", ClientSecret: "work", WebhookSecret: "work"}},
		{personal, DeploymentAppCredentials{PrivateKey: "personal", ClientSecret: "personal", WebhookSecret: "personal"}},
	} {
		if err := repository.SaveRegistration(context.Background(), item.registration, item.credentials); err != nil {
			t.Fatal(err)
		}
	}
	resolved, err := ResolveAppRegistrationConfig(context.Background(), personal.ID, repository)
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Registration == nil || resolved.Registration.ID != personal.ID ||
		resolved.Config.AppID != personal.AppID || resolved.Config.PrivateKey != "personal" {
		t.Fatalf("resolved registration = %+v", resolved)
	}
}
