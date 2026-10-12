import { beforeEach, describe, expect, it, vi } from "vitest";

const fetchJsonMock = vi.hoisted(() => vi.fn());

vi.mock("../client", () => ({
  fetchJson: fetchJsonMock,
}));

import { listFailedInbox } from "./failed-inbox-api";

describe("listFailedInbox", () => {
  beforeEach(() => fetchJsonMock.mockReset());

  it("requests the failed-inbox endpoint with only the workspace id", async () => {
    const response = { rows: [], count: 0, truncated: false };
    fetchJsonMock.mockResolvedValue(response);

    await expect(listFailedInbox("workspace-1")).resolves.toEqual(response);
    expect(fetchJsonMock).toHaveBeenCalledWith(
      "/api/v1/failed-inbox?workspace_id=workspace-1",
      undefined,
    );
  });

  it("encodes the workspace id", async () => {
    fetchJsonMock.mockResolvedValue({ rows: [], count: 0, truncated: false });

    await listFailedInbox("workspace/needs encoding");

    expect(fetchJsonMock).toHaveBeenCalledWith(
      "/api/v1/failed-inbox?workspace_id=workspace%2Fneeds%20encoding",
      undefined,
    );
  });

  it("forwards request options", async () => {
    const options = { cache: "no-store" as const };
    fetchJsonMock.mockResolvedValue({ rows: [], count: 0, truncated: false });

    await listFailedInbox("workspace-1", options);

    expect(fetchJsonMock).toHaveBeenCalledWith(
      "/api/v1/failed-inbox?workspace_id=workspace-1",
      options,
    );
  });
});
