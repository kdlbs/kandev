import type { TokenUsageResponse, TokenUsageRow } from "@/lib/types/http";

export function retainedRow(overrides: Partial<TokenUsageRow> = {}): TokenUsageRow {
  return {
    key: "subject-retained",
    period: "2026-10-12",
    model: "exact-model-id",
    provider: "provider-a",
    task_id: "task-1",
    session_id: "session-1",
    source: "collector-a",
    input_tokens: 100,
    output_tokens: 20,
    cache_read_tokens: 3,
    cache_write_tokens: 4,
    reasoning_tokens: 5,
    total_tokens: 132,
    turns: 1,
    cost_subcents: 12345,
    currency: "USD",
    cost_basis: "reported",
    cost_coverage: "complete",
    effective_cost_per_million: null,
    coverage: "complete",
    estimated: false,
    stale: false,
    undated: false,
    task_available: false,
    session_available: false,
    calendar: "Asia/Kathmandu",
    calendar_basis: "source",
    agent_client: "agent-client-a",
    token_source: { source: "native", actor_status: "mixed" },
    cost_source: { source: "collector-a", actor_status: "unknown" },
    ...overrides,
  };
}

export function retainedReport(overrides: Partial<TokenUsageResponse> = {}): TokenUsageResponse {
  const row = retainedRow();
  return {
    summary: row,
    totals: row,
    rows: [row],
    total_rows: 1,
    has_more: false,
    has_history: true,
    has_range_data: true,
    has_native_history: true,
    dated_coverage: true,
    undated_coverage: false,
    providers: ["provider-a"],
    last_updated: "2026-10-12T09:00:00Z",
    projection_status: "ready",
    projection_generation: "generation-z/opaque",
    projection_pending: false,
    ...overrides,
  };
}

export function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (reason: unknown) => void;
  const promise = new Promise<T>((done, fail) => {
    resolve = done;
    reject = fail;
  });
  return { promise, resolve, reject };
}
