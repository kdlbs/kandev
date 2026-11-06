import { beforeEach, describe, expect, it, vi } from "vitest";

const fetchJsonMock = vi.hoisted(() => vi.fn());

vi.mock("../client", () => ({
  fetchJson: fetchJsonMock,
}));

import { fetchModelUsage, fetchTokenUsage } from "./stats-api";

const WORKSPACE_ID = "workspace-1";

describe("fetchModelUsage", () => {
  beforeEach(() => fetchJsonMock.mockReset());

  it("requests the model-usage endpoint with the selected range", async () => {
    const response = [{ model: "opus", session_count: 2, turn_count: 4, total_duration_ms: 100 }];
    const options = { cache: "no-store" as const };
    fetchJsonMock.mockResolvedValue(response);

    await expect(fetchModelUsage(WORKSPACE_ID, options, "month")).resolves.toEqual(response);
    expect(fetchJsonMock).toHaveBeenCalledWith(
      "/api/v1/workspaces/workspace-1/stats/model-usage?range=month",
      options,
    );
  });
});

describe("fetchTokenUsage", () => {
  beforeEach(() => fetchJsonMock.mockReset());

  it("requests the token usage endpoint with the selected range", async () => {
    const response = {
      summary: {},
      rows: [],
      totals: {},
      has_history: false,
      has_range_data: false,
      dated_coverage: false,
      undated_coverage: false,
    };
    const options = { cache: "no-store" as const };
    fetchJsonMock.mockResolvedValue(response);

    await expect(fetchTokenUsage(WORKSPACE_ID, options, "week")).resolves.toEqual(response);
    expect(fetchJsonMock).toHaveBeenCalledWith(
      "/api/v1/workspaces/workspace-1/stats/token-usage?range=week&include_undated=false",
      options,
    );
  });

  it("sends provider, server sort, and page controls", async () => {
    fetchJsonMock.mockResolvedValue({ rows: [] });
    const options = { cache: "no-store" as const };

    await fetchTokenUsage(WORKSPACE_ID, options, "all", "day", {
      provider: "provider-a",
      sortBy: "period",
      sortDirection: "asc",
      timezone: "Europe/Lisbon",
      limit: 200,
      offset: 400,
    });

    expect(fetchJsonMock).toHaveBeenCalledWith(
      "/api/v1/workspaces/workspace-1/stats/token-usage?range=all&group=day&include_undated=true&timezone=Europe%2FLisbon&provider=provider-a&sort=period&direction=asc&limit=200&offset=400",
      options,
    );
  });
});

describe("token usage exact HTTP amounts", () => {
  beforeEach(() => fetchJsonMock.mockReset());

  it("rejects a rounded integer without its exact decimal companion", async () => {
    fetchJsonMock.mockResolvedValue({
      summary: { total_tokens: 9_007_199_254_740_992 },
      rows: [],
      totals: {},
    });
    await expect(fetchTokenUsage(WORKSPACE_ID)).rejects.toThrow();
  });

  it("preserves exact integer companions and clears unsafe legacy numbers", async () => {
    const metrics = {
      total_tokens: 9_007_199_254_740_992,
      amounts_decimal: { total_tokens: "9007199254740993" },
    };
    fetchJsonMock.mockResolvedValue({
      summary: metrics,
      totals: metrics,
      rows: [{ ...metrics, key: "retained" }],
    });
    const result = await fetchTokenUsage(WORKSPACE_ID);
    expect(result.summary).toEqual({ ...metrics, total_tokens: null });
    expect(result.totals).toEqual(result.summary);
    expect(result.rows[0]).toMatchObject(result.summary);
  });

  it.each(["1.5", "1e18", "-1", "009", "not-a-number", "9223372036854775808"])(
    "rejects invalid decimal amounts: %s",
    async (value) => {
      fetchJsonMock.mockResolvedValue({
        summary: { total_tokens: null, amounts_decimal: { total_tokens: value } },
        totals: {},
        rows: [],
      });
      await expect(fetchTokenUsage(WORKSPACE_ID)).rejects.toThrow();
    },
  );

  it("retains the maximum supported integer and distinguishes unknown from measured zero", async () => {
    const metrics = {
      total_tokens: null,
      input_tokens: 10,
      cost_subcents: 0,
      amounts_decimal: {
        total_tokens: "9223372036854775807",
        input_tokens: null,
        cost_subcents: "0",
      },
    };
    fetchJsonMock.mockResolvedValue({ summary: metrics, totals: metrics, rows: [] });
    const result = await fetchTokenUsage(WORKSPACE_ID);
    expect(result.summary.total_tokens).toBeNull();
    expect(result.summary.amounts_decimal?.total_tokens).toBe("9223372036854775807");
    expect(result.summary.input_tokens).toBeNull();
    expect(result.summary.cost_subcents).toBe(0);
  });

  it("rejects conflicting safe numeric and exact amounts", async () => {
    fetchJsonMock.mockResolvedValue({
      summary: { total_tokens: 7, amounts_decimal: { total_tokens: "8" } },
      totals: {},
      rows: [],
    });
    await expect(fetchTokenUsage(WORKSPACE_ID)).rejects.toThrow();
  });
});
