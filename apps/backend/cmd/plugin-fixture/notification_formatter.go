package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"

	"github.com/kdlbs/kandev/pluginsdk"
)

const fixtureNotificationRevision = "fixture-notification-v1"

func fixtureNotificationDigest() string {
	digest := sha256.Sum256([]byte(fixtureNotificationRevision))
	return hex.EncodeToString(digest[:])
}

func (p *fixturePlugin) FormatNotification(_ context.Context, req *pluginsdk.NotificationFormatRequest) (*pluginsdk.NotificationFormatResponse, error) {
	if req == nil || (!req.Preview && req.ConfigurationRevision != fixtureNotificationRevision) {
		return nil, errors.New("fixture configuration not found")
	}
	return &pluginsdk.NotificationFormatResponse{Custom: true, Title: "Fixture " + req.DefaultTitle, Body: req.DefaultBody}, nil
}

func (p *fixturePlugin) InspectNotificationConfiguration(_ context.Context, req *pluginsdk.NotificationConfigurationRequest) (*pluginsdk.NotificationConfigurationResponse, error) {
	if req == nil || req.Revision != fixtureNotificationRevision {
		return nil, errors.New("fixture configuration not found")
	}
	return &pluginsdk.NotificationConfigurationResponse{Revision: req.Revision, Digest: fixtureNotificationDigest()}, nil
}

func fixturePrepareNotification(req *pluginsdk.PluginActionRequest) (*pluginsdk.PluginActionResponse, error) {
	if req.Context.ActorID == "" || req.Context.NotificationProviderID == "" {
		return nil, errors.New("fixture provider authorization required")
	}
	body, err := json.Marshal(map[string]string{"revision": fixtureNotificationRevision, "digest": fixtureNotificationDigest(), "provider_id": req.Context.NotificationProviderID})
	return &pluginsdk.PluginActionResponse{Body: body, Status: 200}, err
}
