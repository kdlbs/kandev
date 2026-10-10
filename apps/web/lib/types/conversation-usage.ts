export type UsageTokenBreakdown = {
  input_tokens: number;
  cached_read_tokens: number;
  cached_write_tokens: number;
  output_tokens: number;
  thought_tokens: number;
  total_tokens: number;
  output_complete: boolean;
  event_count: number;
  unpriced_count: number;
  cost_subcents?: string;
  overflow: boolean;
};

export type UsageResponse = {
  usage_event_id: string;
  provider_response_id?: string;
  provider_thread_id?: string;
  provider_turn_id?: string;
  scope?: "direct" | "child";
  source?: "response" | "turn_fallback";
  completeness?: "exact" | "estimated" | "partial" | string;
  model?: string;
  provider?: string;
  input_tokens: number;
  cached_read_tokens?: number;
  cached_write_tokens?: number;
  output_tokens?: number;
  thought_tokens?: number;
  reasoning_output_tokens?: number;
  reported_cache_write_tokens?: number;
  reported_total_tokens?: number;
  total_tokens: number;
  cost_subcents?: string;
  cost_source: "provider_reported" | "models_dev_list" | "unpriced" | string;
  estimated: boolean;
  occurred_at: string;
};

export type UsageTurn = {
  turn_id: string;
  completeness: "exact" | "estimated" | "partial" | "unknown" | string;
  direct: UsageTokenBreakdown;
  child: UsageTokenBreakdown;
  total: UsageTokenBreakdown;
  cost_sources: string[];
  last_response?: UsageResponse;
  responses?: UsageResponse[];
};

export type UsageTurnsPage = {
  session_id: string;
  turns: UsageTurn[];
  next_cursor?: string;
};

export type SessionUsageTotals = {
  scope: "session";
  scope_id: string;
  tokens_in: number;
  tokens_cached_read: number;
  tokens_cached_write: number;
  tokens_out: number;
  tokens_thought: number;
  tokens_total: number;
  cost_subcents: number;
  cost_subcents_decimal?: string;
  event_count: number;
  estimated_event_count: number;
  unpriced_event_count: number;
  output_tokens_complete: boolean;
  first_event_at: string | null;
  last_event_at: string | null;
};
