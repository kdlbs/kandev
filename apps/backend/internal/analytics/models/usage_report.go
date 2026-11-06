package models

import (
	"encoding/json"
	"strconv"
)

// UsageReportAccounting is additive HTTP evidence. Actor provenance is separate
// for tokens and cost, even when a report combines both metrics in one row.
type UsageReportAccounting struct {
	TokenSource   *UsageReportSource `json:"token_source"`
	CostSource    *UsageReportSource `json:"cost_source"`
	Calendar      string             `json:"calendar"`
	CalendarBasis string             `json:"calendar_basis"`
}

type UsageReportSource struct {
	Source      string `json:"source"`
	ItemID      string `json:"item_id,omitempty"`
	ActorID     string `json:"actor_id,omitempty"`
	ActorStatus string `json:"actor_status"`
	ActorKind   string `json:"actor_kind"`
}

func exactUsageJSON(value any, amounts map[string]*int64) ([]byte, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &fields); err != nil {
		return nil, err
	}
	decimals := make(map[string]*string, len(amounts))
	for key, amount := range amounts {
		if amount == nil {
			decimals[key] = nil
			continue
		}
		decimal := strconv.FormatInt(*amount, 10)
		decimals[key] = &decimal
		if *amount > 9007199254740991 || *amount < -9007199254740991 {
			fields[key] = json.RawMessage("null")
		}
	}
	fields["amounts_decimal"], err = json.Marshal(decimals)
	if err != nil {
		return nil, err
	}
	return json.Marshal(fields)
}

func (v TokenUsageRow) MarshalJSON() ([]byte, error) {
	type plain TokenUsageRow
	return exactUsageJSON(plain(v), usageReportAmounts(v.InputTokens, v.OutputTokens, v.CacheReadTokens,
		v.CacheWriteTokens, v.ReasoningTokens, v.TotalTokens, v.SourceTotalTokens, v.Turns, v.CostSubcents))
}

func (v UsageTotals) MarshalJSON() ([]byte, error) {
	type plain UsageTotals
	return exactUsageJSON(plain(v), usageReportAmounts(v.InputTokens, v.OutputTokens, v.CacheReadTokens,
		v.CacheWriteTokens, v.ReasoningTokens, v.TotalTokens, v.SourceTotalTokens, v.Turns, v.CostSubcents))
}

func usageReportAmounts(input, output, read, write, reasoning, total, sourceTotal, turns, cost *int64) map[string]*int64 {
	return map[string]*int64{"input_tokens": input, "output_tokens": output, "cache_read_tokens": read,
		"cache_write_tokens": write, "reasoning_tokens": reasoning, "total_tokens": total,
		"source_total_tokens": sourceTotal, "turns": turns, "cost_subcents": cost}
}
