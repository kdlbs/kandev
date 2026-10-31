/**
 * Additive retained-report HTTP mapping from the canonical publication.
 * UsageProjectionState maps to projection_status/generation/pending; generation
 * names the last complete publication and is opaque, including across pages.
 * Every response reads one consistent publication. Later pages supply that
 * token and the resolved range bounds; a changed publication returns 409.
 * Fields remain optional for compatibility with older HTTP responses.
 */
export type UsageReportProjection = {
  projection_status?: string;
  projection_generation?: string;
  projection_pending?: boolean;
  history_gap?: boolean;
  range_start?: string | null;
  range_end?: string | null;
};

export const USAGE_AMOUNT_KEYS = [
  "input_tokens",
  "output_tokens",
  "cache_read_tokens",
  "cache_write_tokens",
  "reasoning_tokens",
  "total_tokens",
  "source_total_tokens",
  "turns",
  "cost_subcents",
] as const;
export type UsageAmountKey = (typeof USAGE_AMOUNT_KEYS)[number];

/**
 * Decimal companions carry exact nonnegative integer amounts. The HTTP mapper
 * must set a legacy numeric field to null above Number.MAX_SAFE_INTEGER.
 * A present null companion means unknown, independently of the numeric field.
 */
export type UsageExactAmounts = {
  amounts_decimal?: Partial<Record<UsageAmountKey, string | null>>;
  /** Original provider total, distinct from the canonical category total. */
  source_total_tokens?: number | null;
};

/** Maps each canonical fact's independent TokenSource / CostSource evidence. */
export type UsageMetricSource = {
  source?: string;
  item_id?: string;
  actor_id?: string;
  actor_status?: string;
  actor_kind?: string;
};

export type UsageAccountingMetadata = {
  token_source?: UsageMetricSource | null;
  cost_source?: UsageMetricSource | null;
  /** Server-selected calendar, never inferred from collection time. */
  calendar?: string;
  /** requested = exact rebucketing; source = retained coarse source calendar. */
  calendar_basis?: "requested" | "source" | "mixed" | "unknown";
};

/**
 * Availability comes from authorized live joins on the retained incarnation.
 * IDs alone never establish availability. Archived live rows remain available.
 * subject_id is the durable identity; reused live IDs must keep distinct keys.
 * Unallocated task/session report rows have empty or omitted IDs. Deleted
 * retained subjects keep their IDs and report unavailable live links.
 */
export type UsageRetainedRow = {
  subject_id?: string;
  task_available?: boolean;
  session_available?: boolean;
  task_archived?: boolean;
  agent_client?: string;
};
