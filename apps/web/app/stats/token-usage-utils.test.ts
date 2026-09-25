import { describe, expect, it } from "vitest";
import type { TokenUsageRow } from "@/lib/types/http";
import {
  aggregateTokenRows,
  aggregateTokenTrendRows,
  calculateEffectiveRate,
  sortTokenRows,
  sumTokenMetrics,
} from "./token-usage-utils";

function usageRow(overrides: Partial<TokenUsageRow> = {}): TokenUsageRow {
  return {
    key: "usage-1",
    period: "2026-09-13",
    model: "model-a",
    provider: "provider-a",
    task_id: "task-1",
    session_id: "session-1",
    input_tokens: 1_000,
    output_tokens: 500,
    cache_read_tokens: 200,
    cache_write_tokens: 50,
    reasoning_tokens: 25,
    total_tokens: 1_775,
    turns: 2,
    cost_subcents: 10_000,
    currency: "USD",
    cost_basis: "estimated",
    cost_coverage: "complete",
    effective_cost_per_million: null,
    source: "plugin:test",
    coverage: "complete",
    estimated: true,
    stale: false,
    undated: false,
    ...overrides,
  };
}

describe("token usage aggregation", () => {
  it("groups rows by model and recomputes the effective rate", () => {
    const rows = aggregateTokenRows(
      [
        usageRow(),
        usageRow({
          period: "2026-09-12",
          task_id: "task-2",
          session_id: "session-2",
          total_tokens: 2_225,
          cost_subcents: 20_000,
        }),
      ],
      "models",
    );

    expect(rows).toHaveLength(1);
    expect(rows[0]).toMatchObject({
      label: "model-a",
      input_tokens: 2_000,
      total_tokens: 4_000,
      cost_subcents: 30_000,
      effective_cost_per_million: 750,
    });
  });

  it("uses source dates for daily and month views", () => {
    const rows = [
      usageRow({ period: "2026-09-13" }),
      usageRow({ period: "2026-09-01", task_id: "task-2" }),
    ];

    expect(aggregateTokenRows(rows, "daily")).toHaveLength(2);
    expect(aggregateTokenRows(rows, "monthly")).toMatchObject([
      { label: "2026-09", period: "2026-09" },
    ]);
  });

  it("groups dated trend points by calendar week", () => {
    const rows = aggregateTokenTrendRows(
      [usageRow({ period: "2026-09-07" }), usageRow({ period: "2026-09-11", task_id: "task-2" })],
      "weekly",
    );

    expect(rows).toHaveLength(1);
    expect(rows[0]).toMatchObject({ label: "2026-09-07", total_tokens: 3_550 });
  });

  it("sorts missing numeric values after known values", () => {
    const rows = aggregateTokenRows(
      [
        usageRow({ model: "known", cost_subcents: 20 }),
        usageRow({ model: "missing", cost_subcents: null }),
      ],
      "models",
    );

    expect(sortTokenRows(rows, "cost_subcents", "desc").map((row) => row.label)).toEqual([
      "known",
      "missing",
    ]);
    expect(sortTokenRows(rows, "cost_subcents", "asc").map((row) => row.label)).toEqual([
      "known",
      "missing",
    ]);
  });

  it("returns no rate when token totals are unavailable", () => {
    expect(calculateEffectiveRate(usageRow({ total_tokens: null }))).toBeNull();
  });

  it("does not recreate a rate for unavailable cost coverage", () => {
    expect(
      calculateEffectiveRate(
        usageRow({ cost_coverage: "missing", effective_cost_per_million: null }),
      ),
    ).toBeNull();
    expect(
      calculateEffectiveRate(usageRow({ cost_basis: "unknown", effective_cost_per_million: null })),
    ).toBeNull();
  });

  it("preserves mixed currency and makes the aggregate rate unavailable", () => {
    const result = sumTokenMetrics([
      usageRow({ currency: "USD" }),
      usageRow({ currency: "EUR", task_id: "task-2" }),
    ]);

    expect(result.currency).toBe("mixed");
    expect(result.effective_cost_per_million).toBeNull();
  });

  it("keeps monthly trend periods in YYYY-MM form", () => {
    expect(aggregateTokenTrendRows([usageRow({ period: "2026-09-13" })], "monthly")).toMatchObject([
      { period: "2026-09", label: "2026-09" },
    ]);
  });
});
