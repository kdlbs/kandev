import { beforeEach, describe, expect, it, vi } from "vitest";

const fetchJsonMock = vi.hoisted(() => vi.fn());

vi.mock("../client", () => ({
  fetchJson: fetchJsonMock,
}));

import { fetchModelUsage } from "./stats-api";

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
