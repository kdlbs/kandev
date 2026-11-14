import { beforeEach, describe, expect, it, vi } from "vitest";

const fetchJsonMock = vi.hoisted(() => vi.fn());

vi.mock("../client", () => ({
  fetchJson: fetchJsonMock,
}));

import { listInboxRecoveryBlocks } from "./clarification-inbox-api";

describe("clarification inbox follow-up reads", () => {
  beforeEach(() => fetchJsonMock.mockReset());

  it("requests the recovery-block list and forwards options", async () => {
    const options = { cache: "no-store" as const };
    fetchJsonMock.mockResolvedValue({ blocks: [], count: 0, truncated: false });
    await listInboxRecoveryBlocks("ws-1", options);
    expect(fetchJsonMock).toHaveBeenCalledWith(
      "/api/v1/clarification-inbox/recovery-blocks?workspace_id=ws-1",
      options,
    );
  });
});
