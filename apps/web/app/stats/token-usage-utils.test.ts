import { describe, expect, it } from "vitest";
import type { TokenUsageRow } from "@/lib/types/http";
import {
  aggregateTokenRows,
  aggregateTokenTrendRows,
  calculateEffectiveRate,
  sortTokenRows,
  sumTokenMetrics,
  formatTokens,
  formatCost,
} from "./token-usage-utils";

const WEEK_START = "2026-09-07";

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
  it("formats every integer digit and fixed-point cost unit without rounding", () => {
    expect(formatTokens(1_234_567, "Unavailable")).toBe("1,234,567");
    expect(formatTokens(9_007_199_254_740_992, "Unavailable")).toBe("Unavailable");
    expect(formatCost(1, "USD", "Unavailable")).toBe("$0.0001");
    expect(formatCost(10_000, "", "Unavailable")).toBe("Unavailable");
    expect(formatTokens(BigInt("9223372036854775807"), "Unavailable")).toBe(
      "9,223,372,036,854,775,807",
    );
    expect(formatCost(BigInt("9223372036854775807"), "EUR", "Unavailable")).toBe(
      "€922,337,203,685,477.5807",
    );
  });

  it("sums large integer companions exactly without rounding through Number", () => {
    const totals = sumTokenMetrics([
      usageRow({ total_tokens: null, amounts_decimal: { total_tokens: "9007199254740993" } }),
      usageRow({ total_tokens: 2 }),
    ]);
    expect(totals.total_tokens).toBeNull();
    expect(totals.amounts_decimal?.total_tokens).toBe("9007199254740995");
  });
  it("groups comparable published rows by model and recomputes the weighted rate", () => {
    const rows = aggregateTokenRows(
      [
        usageRow({ total_tokens: 1_000, effective_cost_per_million: 1_000 }),
        usageRow({
          period: "2026-09-12",
          task_id: "task-2",
          session_id: "session-2",
          total_tokens: 3_000,
          cost_subcents: 20_000,
          effective_cost_per_million: 2_000_000 / 3_000,
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
      [usageRow({ period: WEEK_START }), usageRow({ period: "2026-09-11", task_id: "task-2" })],
      "weekly",
    );

    expect(rows).toHaveLength(1);
    expect(rows[0]).toMatchObject({ label: WEEK_START, total_tokens: 3_550 });
  });
});

describe("published rate availability", () => {
  it.each([
    ["actor identity differs", { actor_id: "user-b" }],
    ["actor status differs", { actor_status: "mixed" }],
    ["actor kind differs", { actor_kind: "agent" }],
    ["rate-matched coverage evidence is unavailable", {}],
  ])("preserves the published null rate when %s", (_reason, costActor) => {
    const actor = { actor_id: "user-a", actor_status: "attributed", actor_kind: "user" };
    const rows = [
      usageRow({
        total_tokens: 1_000,
        cost_subcents: 10_000,
        token_source: { source: "native", ...actor },
        cost_source: { source: "plugin:collector", ...actor, ...costActor },
      }),
      usageRow({
        total_tokens: 3_000,
        cost_subcents: 20_000,
        token_source: { source: "native", ...actor },
        cost_source: { source: "plugin:collector", ...actor, ...costActor },
      }),
    ];
    expect(sumTokenMetrics(rows)).toMatchObject({
      total_tokens: 4_000,
      cost_subcents: 30_000,
      currency: "USD",
      cost_coverage: "complete",
      effective_cost_per_million: null,
    });
    for (const view of ["models", "daily", "monthly", "tasks", "sessions"] as const) {
      expect(aggregateTokenRows(rows, view)).toMatchObject([
        { total_tokens: 4_000, cost_subcents: 30_000, effective_cost_per_million: null },
      ]);
    }
  });

  it.each(["daily", "monthly", "weekly"] as const)(
    "does not restore a withheld rate when merging again into a %s trend",
    (view) => {
      const rows = [
        usageRow({ period: WEEK_START, total_tokens: 1_000, cost_subcents: 10_000 }),
        usageRow({ period: WEEK_START, total_tokens: 3_000, cost_subcents: 20_000 }),
        usageRow({
          period: view === "daily" ? WEEK_START : "2026-09-11",
          total_tokens: 1_000,
          cost_subcents: 5_000,
          effective_cost_per_million: 500,
        }),
      ];
      for (const ordered of [rows, [...rows].reverse()]) {
        expect(aggregateTokenTrendRows(ordered, view)).toMatchObject([
          { total_tokens: 5_000, cost_subcents: 35_000, effective_cost_per_million: null },
        ]);
      }
    },
  );
});

describe("comparable published rates", () => {
  it.each(["daily", "monthly", "weekly"] as const)(
    "aggregates valid published rates by amounts in a %s trend",
    (view) => {
      const rows = [
        usageRow({
          period: WEEK_START,
          total_tokens: 1_000,
          cost_subcents: 10_000,
          effective_cost_per_million: 1_000,
        }),
        usageRow({
          period: view === "daily" ? WEEK_START : "2026-09-11",
          total_tokens: 3_000,
          cost_subcents: 20_000,
          effective_cost_per_million: 2_000_000 / 3_000,
        }),
      ];
      expect(aggregateTokenTrendRows(rows, view)).toMatchObject([
        { total_tokens: 4_000, cost_subcents: 30_000, effective_cost_per_million: 750 },
      ]);
    },
  );

  it("includes a published zero rate in a comparable aggregate", () => {
    expect(
      sumTokenMetrics([
        usageRow({ total_tokens: 1_000, cost_subcents: 0, effective_cost_per_million: 0 }),
        usageRow({ total_tokens: 1_000, cost_subcents: 10_000, effective_cost_per_million: 1_000 }),
      ]),
    ).toMatchObject({
      total_tokens: 2_000,
      cost_subcents: 10_000,
      effective_cost_per_million: 500,
    });
  });
});

describe("token usage sorting and availability", () => {
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

  it("sorts exact large counts without treating decimal companions as missing", () => {
    const rows = aggregateTokenRows(
      [
        usageRow({
          model: "larger",
          total_tokens: null,
          amounts_decimal: { total_tokens: "9007199254740994" },
        }),
        usageRow({
          model: "smaller",
          total_tokens: null,
          amounts_decimal: { total_tokens: "9007199254740993" },
        }),
      ],
      "models",
    );
    expect(sortTokenRows(rows, "total_tokens", "asc").map((row) => row.label)).toEqual([
      "smaller",
      "larger",
    ]);
  });

  it("returns no rate when token totals are unavailable", () => {
    expect(
      calculateEffectiveRate(usageRow({ total_tokens: null, effective_cost_per_million: 1_000 })),
    ).toBeNull();
  });

  it("does not calculate a rate withheld by the server", () => {
    expect(calculateEffectiveRate(usageRow())).toBeNull();
  });

  it("requires a known currency before calculating or merging a rate", () => {
    const row = usageRow({ currency: "", effective_cost_per_million: 1_000 });
    expect(calculateEffectiveRate(row)).toBeNull();
    expect(sumTokenMetrics([row, row]).effective_cost_per_million).toBeNull();
  });

  it("does not recreate a rate for unavailable cost coverage", () => {
    expect(
      calculateEffectiveRate(
        usageRow({ cost_coverage: "missing", effective_cost_per_million: 1_000 }),
      ),
    ).toBeNull();
    expect(
      calculateEffectiveRate(
        usageRow({ cost_basis: "unknown", effective_cost_per_million: 1_000 }),
      ),
    ).toBeNull();
  });

  it("preserves mixed currency and makes the aggregate rate unavailable", () => {
    const result = sumTokenMetrics([
      usageRow({ currency: "USD", effective_cost_per_million: 1_000 }),
      usageRow({ currency: "EUR", task_id: "task-2", effective_cost_per_million: 1_000 }),
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
