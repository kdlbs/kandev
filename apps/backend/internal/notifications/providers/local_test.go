package providers

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
	gatewayws "github.com/kandev/kandev/internal/gateway/websocket"
	"go.uber.org/zap"
)

func TestLocalProviderReturnsNoEligibleSubscriberWhenUserHasNoWebSocketClient(t *testing.T) {
	log, err := logger.NewFromZap(zap.NewNop())
	if err != nil {
		t.Fatalf("create logger: %v", err)
	}
	provider := NewLocalProvider(gatewayws.NewHub(nil, log))
	err = provider.Send(context.Background(), Message{
		EventType:    "system.update_available",
		OccurrenceID: "v1.2.3",
		UserID:       "user-1",
		Title:        "Kandev update available",
		Body:         "Kandev v1.2.3 is available.",
		Payload:      map[string]string{"version": "v1.2.3", "url": "https://example.test/releases/v1.2.3"},
	})
	if !errors.Is(err, ErrNoEligibleSubscriber) {
		t.Fatalf("send error = %v, want no eligible subscriber", err)
	}
}
