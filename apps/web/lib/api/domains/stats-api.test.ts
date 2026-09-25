import { beforeEach, describe, expect, it, vi } from "vitest";

const fetchJsonMock = vi.hoisted(() => vi.fn());

vi.mock("../client", () => ({
  fetchJson: fetchJsonMock,
}));

import { fetchModelUsage, fetchTokenUsage } from "./stats-api";

describe("fetchModelUsage", () => {
  beforeEach(() => fetchJsonMock.mockReset());

  it("requests the model-usage endpoint with the selected range", async () => {
    const response = [{ model: "opus", session_count: 2, turn_count: 4, total_duration_ms: 100 }];
    const options = { cache: "no-store" as const };
    fetchJsonMock.mockResolvedValue(response);

    await expect(fetchModelUsage("workspace-1", options, "month")).resolves.toEqual(response);
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

    await expect(fetchTokenUsage("workspace-1", options, "week")).resolves.toEqual(response);
    expect(fetchJsonMock).toHaveBeenCalledWith(
      "/api/v1/workspaces/workspace-1/stats/token-usage?range=week&include_undated=false",
      options,
    );
  });

  it("sends provider, server sort, and page controls", async () => {
    fetchJsonMock.mockResolvedValue({ rows: [] });
    const options = { cache: "no-store" as const };

    await fetchTokenUsage("workspace-1", options, "all", "day", {
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
