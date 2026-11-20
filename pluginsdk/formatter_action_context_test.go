package pluginsdk

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFormatterActionContextWireRoundTrip(t *testing.T) {
	context := VerifiedActionContext{ActorID: "owner", NotificationProviderID: "mail", NotificationFormatterKey: "second-formatter"}
	require.Equal(t, context, verifiedActionContextFromProto(context.toProto()))
}
