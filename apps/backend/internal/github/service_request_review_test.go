package github

import (
	"context"
	"errors"
	"testing"
	"time"
)

type cacheCountingReviewRequestClient struct {
	*MockClient
	feedbackCalls int
	statusCalls   int
	err           error
}

func TestService_RequestReviewers_InvalidatesOnlyAffectedInFlightPRCaches(t *testing.T) {
	client := &cacheCountingReviewRequestClient{MockClient: NewMockClient()}
	svc := newTestService(client)
	key := prStatusCacheKey("o", "r", 1)
	unrelatedKey := prStatusCacheKey("o", "r", 2)

	for name, cache := range map[string]*ttlCache{
		"feedback": svc.prFeedbackCache,
		"status":   svc.prStatusCache,
	} {
		t.Run(name, func(t *testing.T) {
			targetStarted := make(chan struct{})
			unrelatedStarted := make(chan struct{})
			releaseTarget := make(chan struct{})
			releaseUnrelated := make(chan struct{})
			targetDone := make(chan any, 1)
			unrelatedDone := make(chan any, 1)

			go func() {
				value, _ := cache.doOrFetch(key, func() (any, error) {
					close(targetStarted)
					<-releaseTarget
					return "stale", nil
				})
				targetDone <- value
			}()
			go func() {
				value, _ := cache.doOrFetch(unrelatedKey, func() (any, error) {
					close(unrelatedStarted)
					<-releaseUnrelated
					return "unrelated", nil
				})
				unrelatedDone <- value
			}()

			<-targetStarted
			<-unrelatedStarted
			if err := svc.RequestReviewers(context.Background(), "o", "r", 1, []string{"octocat"}); err != nil {
				t.Fatalf("RequestReviewers: %v", err)
			}

			freshDone := make(chan any, 1)
			go func() {
				value, _ := cache.doOrFetch(key, func() (any, error) {
					return "fresh", nil
				})
				freshDone <- value
			}()
			select {
			case value := <-freshDone:
				if value != "fresh" {
					t.Fatalf("post-invalidation fetch = %v, want fresh", value)
				}
			case <-time.After(2 * time.Second):
				t.Fatal("post-invalidation fetch joined the stale in-flight call")
			}

			close(releaseTarget)
			close(releaseUnrelated)
			<-targetDone
			<-unrelatedDone

			if value, ok := cache.get(key); !ok || value != "fresh" {
				t.Fatalf("affected cache = %v, %t; want fresh, true", value, ok)
			}
			if value, ok := cache.get(unrelatedKey); !ok || value != "unrelated" {
				t.Fatalf("unrelated cache = %v, %t; want unrelated, true", value, ok)
			}
		})
	}
}

func (c *cacheCountingReviewRequestClient) GetPRFeedback(context.Context, string, string, int) (*PRFeedback, error) {
	c.feedbackCalls++
	return &PRFeedback{}, nil
}

func (c *cacheCountingReviewRequestClient) GetPRStatus(context.Context, string, string, int) (*PRStatus, error) {
	c.statusCalls++
	return &PRStatus{}, nil
}

func (c *cacheCountingReviewRequestClient) RequestReviewers(ctx context.Context, owner, repo string, number int, reviewers []string) error {
	if c.err != nil {
		return c.err
	}
	return c.MockClient.RequestReviewers(ctx, owner, repo, number, reviewers)
}

func TestService_RequestReviewers_EvictsOnlyAffectedCachesOnSuccess(t *testing.T) {
	client := &cacheCountingReviewRequestClient{MockClient: NewMockClient()}
	svc := newTestService(client)
	ctx := context.Background()

	for _, number := range []int{1, 2} {
		if _, err := svc.GetPRFeedback(ctx, "o", "r", number); err != nil {
			t.Fatalf("prime feedback: %v", err)
		}
		if _, err := svc.GetPRStatus(ctx, "o", "r", number); err != nil {
			t.Fatalf("prime status: %v", err)
		}
	}
	if err := svc.RequestReviewers(ctx, "o", "r", 1, []string{"octocat"}); err != nil {
		t.Fatalf("RequestReviewers: %v", err)
	}
	for _, number := range []int{1, 2} {
		if _, err := svc.GetPRFeedback(ctx, "o", "r", number); err != nil {
			t.Fatalf("fetch feedback: %v", err)
		}
		if _, err := svc.GetPRStatus(ctx, "o", "r", number); err != nil {
			t.Fatalf("fetch status: %v", err)
		}
	}
	if client.feedbackCalls != 3 || client.statusCalls != 3 {
		t.Fatalf("calls feedback/status = %d/%d, want 3/3", client.feedbackCalls, client.statusCalls)
	}
}

func TestService_RequestReviewers_KeepsCachesOnFailure(t *testing.T) {
	client := &cacheCountingReviewRequestClient{MockClient: NewMockClient(), err: errors.New("GitHub rejected request")}
	svc := newTestService(client)
	ctx := context.Background()
	if _, err := svc.GetPRFeedback(ctx, "o", "r", 1); err != nil {
		t.Fatalf("prime feedback: %v", err)
	}
	if _, err := svc.GetPRStatus(ctx, "o", "r", 1); err != nil {
		t.Fatalf("prime status: %v", err)
	}
	if err := svc.RequestReviewers(ctx, "o", "r", 1, []string{"octocat"}); err == nil {
		t.Fatal("expected request error")
	}
	if _, err := svc.GetPRFeedback(ctx, "o", "r", 1); err != nil {
		t.Fatalf("fetch feedback: %v", err)
	}
	if _, err := svc.GetPRStatus(ctx, "o", "r", 1); err != nil {
		t.Fatalf("fetch status: %v", err)
	}
	if client.feedbackCalls != 1 || client.statusCalls != 1 {
		t.Fatalf("calls feedback/status = %d/%d, want 1/1", client.feedbackCalls, client.statusCalls)
	}
	if requests := client.RequestedReviews(); len(requests) != 0 {
		t.Fatalf("failed request recorded mock pending state: %#v", requests)
	}
}
