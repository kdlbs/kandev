package alertsource

import (
	"context"
	"time"
)

// Source is implemented by every registered alert source. Descriptor
// returns the source's static declaration — T06 registers off this return
// value (D13) — and Normalize turns a raw webhook payload into normalized
// Alerts.
//
// A nil or empty payload is an ERROR, not the legal (nil, nil): an empty
// body is a malformed delivery, while a well-formed body carrying no alerts
// is a heartbeat or a vendor test ping, and conflating the two would report
// a broken vendor integration as a silent success. On any non-nil error the
// caller discards every returned alert; partial results are never accepted.
// (nil, nil) is legal and means "this payload carries no alerts". Result
// order is not meaningful. Normalize must set Status to one of the
// AlertStatusFiring/AlertStatusResolved constants and populate Severity as
// a projection of Labels["severity"].
type Source interface {
	Descriptor() Descriptor
	Normalize(ctx context.Context, payload []byte) ([]Alert, error)
}

// Enricher is an optional capability a Source may additionally implement,
// declared via Capabilities.Enrich and checked by CheckCapabilities. a is
// never nil — that is the caller's obligation, not Enrich's — and cfg is
// always a Config produced by LoadConfig, never a zero value. AC 002.7
// requires a failed enrichment leave the unenriched alert intact; the
// caller (not Enrich) discharges that by passing a's Clone and committing
// it only on a nil error, so an implementation may mutate a freely.
type Enricher interface {
	Enrich(ctx context.Context, cfg Config, a *Alert) error
}

// Poller is an optional capability a Source may additionally implement,
// declared via Capabilities.Poll and checked by CheckCapabilities. Gets the
// same three output clauses as Normalize: a non-nil error discards every
// returned alert, (nil, nil) is legal and means "nothing new since since",
// and result order is not meaningful. since is INCLUSIVE against
// Alert.StartsAt — exclusive would silently and permanently drop any alert
// sharing the boundary timestamp. A zero since means "no lower bound, use
// the source's default window".
type Poller interface {
	Poll(ctx context.Context, cfg Config, since time.Time) ([]Alert, error)
}
