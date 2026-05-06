package acp

import (
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

// TestUsageTracker_CumulativeDelta asserts the codex-acp fallback path:
// usage_update updates the cumulative used counter; consumeUsageDelta
// returns the running total and resets to zero so the next turn starts
// fresh. Cost (when present) is forwarded as the latest snapshot.
func TestUsageTracker_CumulativeDelta(t *testing.T) {
	a := newTestAdapter()

	a.recordUsageDelta("sess-codex", 100, 0)
	a.recordUsageDelta("sess-codex", 350, 0)

	delta, cost := a.consumeUsageDelta("sess-codex")
	if delta != 350 {
		t.Errorf("first consume delta = %d, want 350", delta)
	}
	if cost != 0 {
		t.Errorf("first consume cost = %d, want 0 (no cost reported)", cost)
	}

	// After consume the tracker resets; new turn starts from 0.
	a.recordUsageDelta("sess-codex", 200, 0)
	delta, _ = a.consumeUsageDelta("sess-codex")
	if delta != 200 {
		t.Errorf("second consume delta = %d, want 200 (reset baseline)", delta)
	}
}

// TestUsageTracker_ForwardsCost mirrors claude-acp where usage_update.cost.amount
// is the authoritative per-turn USD figure. The tracker stores the most
// recent value; consume returns it and resets.
func TestUsageTracker_ForwardsCost(t *testing.T) {
	a := newTestAdapter()

	a.recordUsageDelta("sess-claude", 25068, 615) // 0.06156125 USD -> 616 (rounded)
	_, cost := a.consumeUsageDelta("sess-claude")
	if cost != 615 {
		t.Errorf("cost = %d, want 615 (subcents)", cost)
	}

	// Second consume after no updates returns zero (reset).
	_, cost = a.consumeUsageDelta("sess-claude")
	if cost != 0 {
		t.Errorf("second consume cost = %d, want 0", cost)
	}
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
