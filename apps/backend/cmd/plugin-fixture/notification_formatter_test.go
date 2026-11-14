package main

import (
	"context"
	"testing"

	"github.com/kdlbs/kandev/pluginsdk"
	"github.com/stretchr/testify/require"
)

func TestFixtureFormatterIsAContentOnlyOptionalExtension(t *testing.T) {
	fixture := &fixturePlugin{}
	result, err := fixture.FormatNotification(context.Background(), &pluginsdk.NotificationFormatRequest{ConfigurationRevision: "fixture-notification-v1", DefaultTitle: "Original title", DefaultBody: "Original body"})
	require.NoError(t, err)
	require.Equal(t, "Fixture Original title", result.Title)
	require.Equal(t, "Original body", result.Body)
	_, err = fixture.InspectNotificationConfiguration(context.Background(), &pluginsdk.NotificationConfigurationRequest{Revision: "missing"})
	require.Error(t, err)
}
