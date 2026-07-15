package service

import (
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/notifications/models"
	"go.uber.org/zap"
)

func TestNewServiceSuppressesSystemProviderForDesktopOwnedLaunch(t *testing.T) {
	t.Setenv("KANDEV_DESKTOP_NATIVE_NOTIFICATIONS", "true")
	log, err := logger.NewFromZap(zap.NewNop())
	if err != nil {
		t.Fatalf("create logger: %v", err)
	}

	svc := NewService(nil, nil, nil, log)

	if _, exists := svc.providers[models.ProviderTypeSystem]; exists {
		t.Fatal("system notification provider must be suppressed for a desktop-owned launch")
	}
	if _, exists := svc.providers[models.ProviderTypeLocal]; !exists {
		t.Fatal("local websocket notification provider must remain enabled")
	}
}

func TestNewServiceRetainsSystemProviderForNonDesktopLaunch(t *testing.T) {
	t.Setenv("KANDEV_DESKTOP_NATIVE_NOTIFICATIONS", "")
	log, err := logger.NewFromZap(zap.NewNop())
	if err != nil {
		t.Fatalf("create logger: %v", err)
	}

	svc := NewService(nil, nil, nil, log)

	if _, exists := svc.providers[models.ProviderTypeSystem]; !exists {
		t.Fatal("system notification provider must remain enabled outside the desktop-owned launch")
	}
}
