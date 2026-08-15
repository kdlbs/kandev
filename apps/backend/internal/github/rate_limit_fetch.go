package github

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// rateLimitResponse mirrors the GitHub GET /rate_limit JSON shape. The
// endpoint includes additional resource buckets, but these are the buckets
// Kandev tracks and displays.
type rateLimitResponse struct {
	Resources rateLimitResources `json:"resources"`
}

type rateLimitResources struct {
	Core    rateLimitBucket `json:"core"`
	GraphQL rateLimitBucket `json:"graphql"`
	Search  rateLimitBucket `json:"search"`
}

type rateLimitBucket struct {
	Limit     int   `json:"limit"`
	Remaining int   `json:"remaining"`
	Reset     int64 `json:"reset"`
}

func recordRateLimitResources(tracker *RateTracker, resources rateLimitResources) {
	if tracker == nil {
		return
	}
	now := time.Now().UTC()
	record := func(resource Resource, bucket rateLimitBucket) {
		if bucket.Limit == 0 && bucket.Remaining == 0 && bucket.Reset == 0 {
			return
		}
		tracker.Record(RateSnapshot{
			Resource:  resource,
			Limit:     bucket.Limit,
			Remaining: bucket.Remaining,
			ResetAt:   time.Unix(bucket.Reset, 0).UTC(),
			UpdatedAt: now,
		})
	}
	record(ResourceCore, resources.Core)
	record(ResourceGraphQL, resources.GraphQL)
	record(ResourceSearch, resources.Search)
}

// FetchRateLimit calls GET /rate_limit and seeds the client's tracker with
// the current REST, GraphQL, and search buckets.
func (c *TokenClient) FetchRateLimit(ctx context.Context) error {
	if c.rateTracker == nil {
		return nil
	}
	var raw rateLimitResponse
	if err := c.get(ctx, "/rate_limit", &raw); err != nil {
		return fmt.Errorf("fetch rate limit: %w", err)
	}
	recordRateLimitResources(c.rateTracker, raw.Resources)
	return nil
}

func decodeRateLimitResponse(data []byte) (rateLimitResponse, error) {
	var raw rateLimitResponse
	if err := json.Unmarshal(data, &raw); err != nil {
		return rateLimitResponse{}, err
	}
	return raw, nil
}
