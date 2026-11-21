import { beforeEach, describe, expect, it, vi } from "vitest";
import { ApiError } from "@/lib/api/client";

const api = vi.hoisted(() => ({
  listClarificationInbox: vi.fn(),
  listUnansweredClarificationInbox: vi.fn(),
  listInboxRecoveryBlocks: vi.fn(),
  listFailedInbox: vi.fn(),
}));

vi.mock("@/lib/api/domains/clarification-inbox-api", () => ({
  listClarificationInbox: api.listClarificationInbox,
  listUnansweredClarificationInbox: api.listUnansweredClarificationInbox,
  listInboxRecoveryBlocks: api.listInboxRecoveryBlocks,
}));
vi.mock("@/lib/api/domains/failed-inbox-api", () => ({ listFailedInbox: api.listFailedInbox }));

import { readWorkspaceInbox } from "./use-control-inbox";

describe("readWorkspaceInbox", () => {
  beforeEach(() => {
    for (const fn of Object.values(api)) fn.mockReset();
  });

  it("reads all four sections of one workspace with the given options", async () => {
    const bundle = { pending_id: "p1" };
    api.listClarificationInbox.mockResolvedValue({ bundles: [bundle], count: 1, next_cursor: "c" });
    api.listUnansweredClarificationInbox.mockResolvedValue({
      enabled: true,
      retention_days: 7,
      bundles: [],
      count: 0,
    });
    api.listInboxRecoveryBlocks.mockResolvedValue({
      blocks: [{ block_id: "b1" }],
      count: 1,
      truncated: false,
    });
    api.listFailedInbox.mockResolvedValue({ rows: [], count: 0, truncated: true });
    const options = { cache: "no-store" as const };

    const result = await readWorkspaceInbox("w1", options);

    expect(api.listClarificationInbox).toHaveBeenCalledWith("w1", options);
    expect(api.listUnansweredClarificationInbox).toHaveBeenCalledWith("w1", options);
    expect(result).toEqual({
      workspaceId: "w1",
      answerable: { status: "ready", items: [bundle], truncated: true },
      earlier: { status: "ready", items: [], truncated: false },
      recovery: { status: "ready", items: [{ block_id: "b1" }], truncated: false },
      failed: { status: "ready", items: [], truncated: true },
    });
  });

  it("lists no superseded questions while the workspace's unanswered setting is off", async () => {
    api.listClarificationInbox.mockResolvedValue({ bundles: [], count: 0 });
    api.listUnansweredClarificationInbox.mockResolvedValue({
      enabled: false,
      retention_days: 7,
      bundles: [{ pending_id: "stale" }],
      count: 1,
    });
    api.listInboxRecoveryBlocks.mockResolvedValue({ blocks: [], count: 0, truncated: false });
    api.listFailedInbox.mockResolvedValue({ rows: [], count: 0, truncated: false });

    const result = await readWorkspaceInbox("w1", {});

    expect(result.earlier).toEqual({ status: "ready", items: [], truncated: false });
  });

  it("keeps the other sections when one fails, classifying the failure", async () => {
    api.listClarificationInbox.mockResolvedValue({ bundles: [], count: 0 });
    api.listUnansweredClarificationInbox.mockRejectedValue(new ApiError("missing", 404, null));
    api.listInboxRecoveryBlocks.mockRejectedValue(new ApiError("denied", 403, null));
    api.listFailedInbox.mockRejectedValue(new Error("network"));

    const result = await readWorkspaceInbox("w1", {});

    expect(result.answerable.status).toBe("ready");
    expect(result.earlier).toEqual({ status: "unsupported", items: [], truncated: false });
    expect(result.recovery.status).toBe("denied");
    expect(result.failed.status).toBe("error");
  });
});
