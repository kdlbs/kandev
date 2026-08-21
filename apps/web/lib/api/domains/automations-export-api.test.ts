import { beforeEach, describe, expect, it, vi } from "vitest";

const mockFetchBlob = vi.fn();
vi.mock("../client", () => ({
  fetchBlob: (...args: unknown[]) => mockFetchBlob(...args),
}));

import { exportAutomationsZip } from "./automations-export-api";

beforeEach(() => {
  mockFetchBlob.mockReset();
});

describe("exportAutomationsZip", () => {
  it("fetches the workspace's zip export endpoint", async () => {
    const blob = new Blob(["zip-bytes"], { type: "application/zip" });
    mockFetchBlob.mockResolvedValue(blob);

    const result = await exportAutomationsZip("ws-1");

    expect(mockFetchBlob).toHaveBeenCalledWith("/api/v1/workspaces/ws-1/automations/export/zip");
    expect(result).toBe(blob);
  });
});
