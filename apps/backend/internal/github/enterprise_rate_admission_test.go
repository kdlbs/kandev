package github

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/githubhost"
	"github.com/stretchr/testify/require"
)

func TestEnterprisePaginationUsesSearchAdmission(t *testing.T) {
	for _, hostName := range []string{"github.com", "tenant.ghe.com", "git.example.com"} {
		t.Run(hostName, func(t *testing.T) {
			host, err := githubhost.Parse(hostName)
			require.NoError(t, err)
			client := NewPATClientForHost("fixture", host)
			coordinator := NewRateCoordinator(nil, testLogger(t))
			tracker, admission := coordinator.coordinate(hostName, AuthPrincipal{Kind: AuthPrincipalHuman, Login: "alice"}, nil)
			client.WithRateTracker(tracker).withRateAdmission(admission)
			tracker.ObservePrimary(ResourceSearch, time.Now().Add(time.Hour), RetrySourcePrimaryReset)
			requests := 0
			client.httpClient.Transport = enterpriseTransportFunc(func(*http.Request) (*http.Response, error) {
				requests++
				return nil, context.Canceled
			})
			var result map[string]any
			_, err = client.getPaginated(pollerContext(t.Context()), host.RESTURL()+"/search/issues?page=2", &result)
			var deferred *AdmissionDeferredError
			require.ErrorAs(t, err, &deferred)
			require.Equal(t, ResourceSearch, deferred.Resource)
			require.Zero(t, requests, "a paginated search must wait for the search quota")
		})
	}
}
