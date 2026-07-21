package gitlab

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type failingHostStore struct{}

func (failingHostStore) GetHost(context.Context) (string, error) {
	return "", errors.New("settings unavailable")
}

func (failingHostStore) SetHost(context.Context, string) error {
	return nil
}

func TestProvideFailsClosedWhenHostStoreCannotBeRead(t *testing.T) {
	t.Setenv("KANDEV_MOCK_GITLAB", "true")
	t.Setenv("GITLAB_TOKEN", "token-for-self-managed-host")

	svc, cleanup, err := Provide(context.Background(), nil, failingHostStore{}, newTestLogger(t))
	if err == nil {
		t.Fatal("expected host store read error")
	}
	if !strings.Contains(err.Error(), "load GitLab host") {
		t.Fatalf("err = %v, want load GitLab host context", err)
	}
	if svc != nil {
		t.Fatalf("service = %#v, want nil", svc)
	}
	if cleanup != nil {
		t.Fatalf("cleanup = %T, want nil", cleanup)
	}
}
