import { describe, expect, it } from "vitest";
import type { NativeModel } from "@/lib/types/control-native";
import { modelTotals, visibleTokenFields } from "./control-model-totals";

const model = (over: Partial<NativeModel> = {}): NativeModel => ({
  model: "opus",
  provider: "anthropic",
  measurement_source: "unknown",
  usage_completeness: "unknown",
  cost_source: "provider_reported",
  estimated: false,
  event_count: 1,
  tokens_in: 0,
  tokens_out: null,
  tokens_cached_read: null,
  tokens_cached_write: null,
  tokens_total: 0,
  cost_subcents: 100,
  ...over,
});

describe("model totals", () => {
  it("sums one model across workspaces and keeps the workspace split", () => {
    const rows = modelTotals([
      { workspaceId: "a", models: [model({ event_count: 2, cost_subcents: 229_189 })] },
      { workspaceId: "b", models: [model({ event_count: 3, cost_subcents: 11 })] },
      { workspaceId: "b", models: [model({ model: "flash", provider: "", cost_subcents: 0 })] },
    ]);
    expect(rows.map((row) => [row.model, row.events, row.cost])).toEqual([
      ["opus", 5, { value: 229_200, partial: false }],
      ["flash", 1, { value: 0, partial: false }],
    ]);
    expect(rows[0].workspaces.map((share) => [share.workspaceId, share.events])).toEqual([
      ["a", 2],
      ["b", 3],
    ]);
    expect(rows[1].providers).toEqual([]);
  });
  it("keeps unreported measurements unknown and hides token columns nobody reported", () => {
    const rows = modelTotals([{ workspaceId: "a", models: [model({ cost_source: "unpriced" })] }]);
    expect(rows[0].tokens.tokens_in).toEqual({ value: null, partial: false });
    expect(rows[0].cost).toEqual({ value: null, partial: false });
    expect(visibleTokenFields(rows)).toEqual([]);
  });
  it("marks a sum partial when only some records reported it", () => {
    const measured = { measurement_source: "provider", usage_completeness: "exact" };
    const rows = modelTotals([
      { workspaceId: "a", models: [model({ ...measured, tokens_in: 7, tokens_out: 3 })] },
      { workspaceId: "b", models: [model({ cost_source: "unknown" })] },
    ]);
    expect(rows[0].tokens.tokens_in).toEqual({ value: 7, partial: true });
    expect(rows[0].cost).toEqual({ value: 100, partial: true });
    expect(visibleTokenFields(rows)).toEqual(["tokens_in", "tokens_out", "tokens_total"]);
  });
  it("groups a dynamic policy name with an absent model rather than showing it", () => {
    const rows = modelTotals([
      { workspaceId: "a", models: [model({ model: "Dynamic" }), model({ model: "" })] },
    ]);
    expect(rows.map((row) => [row.model, row.events])).toEqual([["", 2]]);
  });
});
