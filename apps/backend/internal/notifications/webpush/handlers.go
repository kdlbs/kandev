package webpush

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
)

// ProviderEnsurer makes sure the user has a Web Push notification provider
// (and therefore a row in the notification-events settings table).
type ProviderEnsurer interface {
	EnsureWebPushProvider(ctx context.Context, userID string) error
}

// UserResolver maps a request context to the user whose devices it may touch.
type UserResolver func(ctx context.Context) string

type handlers struct {
	svc      *Service
	ensurer  ProviderEnsurer
	userID   UserResolver
	logger   *logger.Logger
	testCopy testCopy
}

// testCopy is the fixed text of the diagnostic push. It is plain English for
// the same reason the notification service's test message is: it is produced
// on the server and carries no user data.
type testCopy struct{ title, body string }

// RegisterRoutes mounts the Web Push endpoints on the authenticated API group.
func RegisterRoutes(router *gin.Engine, svc *Service, ensurer ProviderEnsurer, userID UserResolver, log *logger.Logger) {
	h := &handlers{
		svc:      svc,
		ensurer:  ensurer,
		userID:   userID,
		logger:   log.WithFields(zap.String("component", "webpush-handlers")),
		testCopy: testCopy{title: "Test notification", body: "Push notifications are working on this device."},
	}
	api := router.Group("/api/v1/push")
	api.GET("/public-key", h.publicKey)
	api.GET("/subscriptions", h.list)
	api.POST("/subscriptions", h.subscribe)
	api.DELETE("/subscriptions", h.unsubscribe)
	api.POST("/test", h.test)
}

type subscribeRequest struct {
	Endpoint string `json:"endpoint"`
	Keys     struct {
		P256dh string `json:"p256dh"`
		Auth   string `json:"auth"`
	} `json:"keys"`
}

type unsubscribeRequest struct {
	Endpoint string `json:"endpoint"`
}

type subscriptionDTO struct {
	Endpoint   string  `json:"endpoint"`
	UserAgent  string  `json:"user_agent"`
	CreatedAt  string  `json:"created_at"`
	LastUsedAt *string `json:"last_used_at,omitempty"`
}

func (h *handlers) publicKey(c *gin.Context) {
	key, err := h.svc.PublicKey(c.Request.Context())
	if err != nil {
		h.logger.Error("failed to load VAPID public key", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "push is unavailable"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"public_key": key})
}

func (h *handlers) list(c *gin.Context) {
	subs, err := h.svc.Subscriptions(c.Request.Context(), h.userID(c.Request.Context()))
	if err != nil {
		h.logger.Error("failed to list push subscriptions", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list push subscriptions"})
		return
	}
	out := make([]subscriptionDTO, 0, len(subs))
	for _, sub := range subs {
		dto := subscriptionDTO{
			Endpoint:  sub.Endpoint,
			UserAgent: sub.UserAgent,
			CreatedAt: sub.CreatedAt.UTC().Format(time.RFC3339),
		}
		if sub.LastUsedAt != nil {
			s := sub.LastUsedAt.UTC().Format(time.RFC3339)
			dto.LastUsedAt = &s
		}
		out = append(out, dto)
	}
	c.JSON(http.StatusOK, gin.H{"subscriptions": out})
}

func (h *handlers) subscribe(c *gin.Context) {
	var body subscribeRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}
	ctx := c.Request.Context()
	userID := h.userID(ctx)
	_, err := h.svc.Subscribe(ctx, userID, SubscribeInput{
		Endpoint:  body.Endpoint,
		P256dh:    body.Keys.P256dh,
		Auth:      body.Keys.Auth,
		UserAgent: c.Request.UserAgent(),
	})
	if errors.Is(err, ErrInvalidSubscription) {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err != nil {
		h.logger.Error("failed to store push subscription", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to store push subscription"})
		return
	}
	if h.ensurer != nil {
		if err := h.ensurer.EnsureWebPushProvider(ctx, userID); err != nil {
			h.logger.Error("failed to ensure web push provider", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to enable push notifications"})
			return
		}
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

func (h *handlers) unsubscribe(c *gin.Context) {
	var body unsubscribeRequest
	if err := c.ShouldBindJSON(&body); err != nil || body.Endpoint == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}
	err := h.svc.Unsubscribe(c.Request.Context(), h.userID(c.Request.Context()), body.Endpoint)
	if errors.Is(err, ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "subscription not found"})
		return
	}
	if err != nil {
		h.logger.Error("failed to remove push subscription", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to remove push subscription"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

func (h *handlers) test(c *gin.Context) {
	ctx := c.Request.Context()
	delivered, err := h.svc.Notify(ctx, h.userID(ctx), Payload{
		Title: h.testCopy.title,
		Body:  h.testCopy.body,
		Tag:   "kandev-test",
		URL:   "/settings/notifications",
	}, SendOptions{Urgency: UrgencyHigh, TTL: time.Minute})
	if err != nil {
		h.logger.Warn("test push failed", zap.Error(err))
		c.JSON(http.StatusBadGateway, gin.H{"error": "the push service rejected the test notification"})
		return
	}
	if delivered == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "no push subscriptions registered"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "delivered": delivered})
}
