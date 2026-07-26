package acp

import (
	"context"
	"testing"

	"github.com/coder/acp-go-sdk"
)

// TestExtractUsage walks the three usage shapes observed in
// /tmp/acp-probe-*.jsonl and confirms extractUsage picks each up.
func TestExtractUsage(t *testing.T) {
	intp := func(v int) *int { return &v }

	tests := []struct {
		name    string
		resp    *acp.PromptResponse
		wantIn  int64
		wantOut int64
		wantCR  int64
		wantCW  int64
		wantTh  int64
		wantNil bool
	}{
		{
			name: "claude-acp result.usage (typed Usage)",
			resp: &acp.PromptResponse{
				Usage: &acp.Usage{
					InputTokens:       6,
					OutputTokens:      7,
					CachedReadTokens:  intp(16634),
					CachedWriteTokens: intp(8421),
					TotalTokens:       25068,
				},
			},
			wantIn: 6, wantOut: 7, wantCR: 16634, wantCW: 8421,
		},
		{
			name: "opencode-acp result.usage with thoughtTokens",
			resp: &acp.PromptResponse{
				Usage: &acp.Usage{
					InputTokens:   10639,
					OutputTokens:  2,
					ThoughtTokens: intp(11),
					TotalTokens:   10652,
				},
			},
			wantIn: 10639, wantOut: 2, wantTh: 11,
		},
		{
			name: "gemini _meta.quota.model_usage[].token_count",
			resp: &acp.PromptResponse{
				Meta: map[string]any{
					"quota": map[string]any{
						"model_usage": []any{
							map[string]any{
								"model": "gemini-3-flash-preview",
								"token_count": map[string]any{
									"input_tokens":  float64(9796),
									"output_tokens": float64(2),
								},
							},
						},
					},
				},
			},
			wantIn: 9796, wantOut: 2,
		},
		{
			name: "gemini _meta.quota.token_count flat",
			resp: &acp.PromptResponse{
				Meta: map[string]any{
					"quota": map[string]any{
						"token_count": map[string]any{
							"input_tokens":  float64(100),
							"output_tokens": float64(50),
						},
					},
				},
			},
			wantIn: 100, wantOut: 50,
		},
		{
			name: "_meta.usage legacy snake_case fallback",
			resp: &acp.PromptResponse{
				Meta: map[string]any{
					"usage": map[string]any{
						"input_tokens":  float64(42),
						"output_tokens": float64(7),
						"total_tokens":  float64(49),
					},
				},
			},
			wantIn: 42, wantOut: 7,
		},
		{
			name: "_meta.usage legacy camelCase fallback",
			resp: &acp.PromptResponse{
				Meta: map[string]any{
					"usage": map[string]any{
						"inputTokens":  float64(5),
						"outputTokens": float64(3),
						"totalTokens":  float64(8),
					},
				},
			},
			wantIn: 5, wantOut: 3,
		},
		{
			name:    "empty response yields nil",
			resp:    &acp.PromptResponse{},
			wantNil: true,
		},
		{
			name:    "nil response yields nil",
			resp:    nil,
			wantNil: true,
		},
		{
			name: "all-zero typed usage falls through and yields nil",
			resp: &acp.PromptResponse{
				Usage: &acp.Usage{},
			},
			wantNil: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := extractUsage(tc.resp)
			if tc.wantNil {
				if got != nil {
					t.Fatalf("expected nil, got %#v", got)
				}
				return
			}
			if got == nil {
				t.Fatal("expected non-nil usage")
			}
			if got.InputTokens != tc.wantIn {
				t.Errorf("InputTokens = %d, want %d", got.InputTokens, tc.wantIn)
			}
			if got.OutputTokens != tc.wantOut {
				t.Errorf("OutputTokens = %d, want %d", got.OutputTokens, tc.wantOut)
			}
			if got.CachedReadTokens != tc.wantCR {
				t.Errorf("CachedReadTokens = %d, want %d", got.CachedReadTokens, tc.wantCR)
			}
			if got.CachedWriteTokens != tc.wantCW {
				t.Errorf("CachedWriteTokens = %d, want %d", got.CachedWriteTokens, tc.wantCW)
			}
			if got.ThoughtTokens != tc.wantTh {
				t.Errorf("ThoughtTokens = %d, want %d", got.ThoughtTokens, tc.wantTh)
			}
		})
	}
}

func TestExtractUsage_DoesNotInterpretGrokPrivateReasoningTokens(t *testing.T) {
	usage := extractUsage(&acp.PromptResponse{Meta: map[string]any{
		"usage": map[string]any{
			"inputTokens":     float64(5),
			"outputTokens":    float64(3),
			"totalTokens":     float64(8),
			"reasoningTokens": float64(2),
		},
	}})
	if usage == nil {
		t.Fatal("expected non-nil usage")
	}
	if usage.ThoughtTokens != 0 {
		t.Fatalf("ThoughtTokens = %d, want 0; Grok private fields belong to its ACP dialect", usage.ThoughtTokens)
	}
}

// TestUsageTracker_CumulativeDelta asserts the codex-acp fallback path:
// usage_update reports current context occupancy; consumeUsageDelta returns
// nonnegative growth since the previously consumed baseline.
func TestUsageTracker_CumulativeDelta(t *testing.T) {
	a := newTestAdapter()
	const sess = "sess-codex"

	a.convertUsageUpdate(sess, usageUpdate(200_000, 100))
	a.convertUsageUpdate(sess, usageUpdate(200_000, 350))

	delta, cost := a.consumeUsageDelta(sess)
	if delta != 350 {
		t.Errorf("first consume delta = %d, want 350", delta)
	}
	if cost != 0 {
		t.Errorf("first consume cost = %d, want 0 (no cost reported)", cost)
	}

	// Compaction decreases occupancy. It resets both baselines instead of
	// emitting a negative delta or recounting the restored context.
	a.convertUsageUpdate(sess, usageUpdate(200_000, 200))
	delta, _ = a.consumeUsageDelta(sess)
	if delta != 0 {
		t.Errorf("post-compaction delta = %d, want 0", delta)
	}
	a.convertUsageUpdate(sess, usageUpdate(200_000, 260))
	delta, _ = a.consumeUsageDelta(sess)
	if delta != 60 {
		t.Errorf("growth after compaction = %d, want 60", delta)
	}
}

// TestUsageTracker_CumulativeUSDCost mirrors claude-acp where
// usage_update.cost.amount is authoritative cumulative USD session cost.
func TestUsageTracker_CumulativeUSDCost(t *testing.T) {
	a := newTestAdapter()
	const sess = "sess-claude"

	a.convertUsageUpdate(sess, usageUpdateWithCost(200_000, 25_068, 0.06156125, "USD"))
	_, cost := a.consumeUsageDelta(sess)
	if cost != 615 {
		t.Errorf("cost = %d, want 615 (subcents)", cost)
	}

	a.convertUsageUpdate(sess, usageUpdateWithCost(200_000, 25_500, 0.07156125, "USD"))
	_, cost = a.consumeUsageDelta(sess)
	if cost != 100 {
		t.Errorf("cumulative cost delta = %d, want 100", cost)
	}

	_, cost = a.consumeUsageDelta(sess)
	if cost != 0 {
		t.Errorf("second consume cost = %d, want 0", cost)
	}
}

func TestUsageTracker_IgnoresNonUSDCostAndResetsDecreases(t *testing.T) {
	a := newTestAdapter()
	const sess = "sess-cost-currency"

	a.convertUsageUpdate(sess, usageUpdateWithCost(200_000, 100, 2.5, "EUR"))
	_, cost := a.consumeUsageDelta(sess)
	if cost != 0 {
		t.Fatalf("EUR cost = %d, want 0", cost)
	}

	a.convertUsageUpdate(sess, usageUpdateWithCost(200_000, 110, 0.05, "USD"))
	_, cost = a.consumeUsageDelta(sess)
	if cost != 500 {
		t.Fatalf("initial USD cost = %d, want 500", cost)
	}
	a.convertUsageUpdate(sess, usageUpdateWithCost(200_000, 120, 0.01, "USD"))
	_, cost = a.consumeUsageDelta(sess)
	if cost != 0 {
		t.Fatalf("decreased USD cumulative cost = %d, want 0", cost)
	}
}

func usageUpdateWithCost(size, used int64, amount float64, currency string) *acp.SessionUsageUpdate {
	update := usageUpdate(size, used)
	update.Cost = &acp.Cost{Amount: amount, Currency: currency}
	return update
}

// TestConsumeUsageDelta_UnknownSession returns zero for sessions that
// never recorded usage (e.g. claude-acp where the typed resp.Usage is
// the authoritative path and the tracker was never touched).
func TestConsumeUsageDelta_UnknownSession(t *testing.T) {
	a := newTestAdapter()
	d, c := a.consumeUsageDelta("never-seen")
	if d != 0 || c != 0 {
		t.Errorf("unknown session = (%d, %d), want (0, 0)", d, c)
	}
}

func TestNewSession_ClearsUsageTrackers(t *testing.T) {
	a, _ := setupConcurrencyFakeAgent(t)
	if err := a.Initialize(context.Background()); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	a.convertUsageUpdate("old-session", usageUpdate(200_000, 500))

	if _, err := a.NewSession(context.Background(), nil); err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	if got := len(a.usageBySession); got != 0 {
		t.Fatalf("usage tracker count = %d, want 0", got)
	}
}
