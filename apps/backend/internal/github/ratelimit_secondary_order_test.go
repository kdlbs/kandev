package github

import (
	"context"
	"sync"
	"testing"
	"testing/synctest"
	"time"
)

// @covers AC-INTEGRATIONS-GITHUB-RATE-001.4
// @covers AC-INTEGRATIONS-GITHUB-RATE-001.5
func TestSecondaryRefusalsPreserveLatestBoundary(t *testing.T) {
	for _, order := range []string{"long-first", "short-first", "concurrent", "equal"} {
		t.Run(order, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				tracker, admission := NewRateCoordinator(nil, nil).coordinate("github.com", AuthPrincipal{Kind: AuthPrincipalHuman, Login: "alice"}, nil)
				later := time.Now().Add(10 * time.Minute)
				short := time.Now().Add(time.Minute)
				if order == "equal" {
					short = later
				}
				longRefusal := func() { tracker.ObserveSecondary(ResourceCore, later, RetrySourceRetryAfter, "") }
				shortRefusal := func() { tracker.ObserveSecondary(ResourceCore, short, RetrySourceConservativeFallback, "") }
				switch order {
				case "short-first":
					shortRefusal()
					longRefusal()
				case "concurrent":
					var wg sync.WaitGroup
					for _, f := range []func(){longRefusal, shortRefusal} {
						wg.Go(f)
					}
					wg.Wait()
				default:
					longRefusal()
					shortRefusal()
				}
				state := tracker.Secondary(ResourceCore)
				if !state.RetryAt.Equal(later) || state.RetrySource != RetrySourceRetryAfter {
					t.Fatalf("secondary = %+v, want deadline %s from Retry-After", state, later)
				}
				time.Sleep(2 * time.Minute)
				release, err := admission.acquire(pollerContext(context.Background()), ResourceCore)
				if release != nil {
					release()
				}
				if err == nil {
					t.Fatal("admitted before the longest Retry-After expired")
				}
				tracker.ObserveSecondary(ResourceSearch, later, RetrySourceRetryAfter, "")
				tracker.ObserveSuccess(ResourceCore)
				if tracker.Secondary(ResourceCore).Active || !tracker.Secondary(ResourceSearch).Active {
					t.Fatal("success must clear only the successful resource")
				}
				time.Sleep(9 * time.Minute)
				if tracker.Secondary(ResourceSearch).Active {
					t.Fatal("expired secondary boundary remains active")
				}
			})
		})
	}
}
