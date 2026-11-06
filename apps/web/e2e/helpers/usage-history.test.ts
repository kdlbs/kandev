import { describe, expect, it, vi } from "vitest";
import type { Page } from "@playwright/test";
import { readUsage, usageTarget, waitForUsage, type UsageTarget } from "./usage-history";
import type { TokenUsageResponse } from "../../lib/types/http";

vi.mock("@playwright/test", async () => ({
  expect: (await import("vitest")).expect,
}));

const target = {
  SubjectBinding: "host-issued-binding",
  TaskID: "task",
  SessionID: "session",
  SourceScopeStatus: "eligible",
  ActorAttributionStatus: "attributed",
} satisfies UsageTarget & { SourceScopeStatus: string };

type TargetPage = {
  Targets: (UsageTarget & { SourceScopeStatus: string })[] | null;
  SnapshotID: string;
  NextCursor: string;
  HasMore: boolean;
  Preparing: boolean;
};

function pageReply(overrides: Partial<TargetPage> = {}): TargetPage {
  return {
    Targets: null,
    SnapshotID: "",
    NextCursor: "",
    HasMore: false,
    Preparing: false,
    ...overrides,
  };
}

function hostPages(pages: TargetPage[], status = 200) {
  const post = vi.fn(async (_url: string, _options: { data: unknown }) => {
    const value = pages.shift();
    if (!value) throw new Error("Unexpected discovery request");
    return {
      status: () => status,
      text: async () => (status === 200 ? JSON.stringify(value) : "scope denied"),
      json: async () => value,
    };
  });
  return { page: { request: { post } } as unknown as Page, post };
}

function usageResponse(status: number, body: unknown) {
  const responseBody = JSON.stringify(body);
  return {
    status: () => status,
    text: async () => responseBody,
    json: async () => body,
  };
}

function usagePages(responses: ReturnType<typeof usageResponse>[]) {
  const get = vi.fn(async (_url: string) => {
    const response = responses.shift();
    if (!response) throw new Error("Unexpected usage read");
    return response;
  });
  return { page: { request: { get } } as unknown as Page, get };
}

const readyUsage = {
  projection_status: "ready",
  projection_pending: false,
  projection_generation: "generation-1",
  totals: { amounts_decimal: { total_tokens: "41" } },
} as TokenUsageResponse;

describe("usageTarget Host discovery", () => {
  it("starts fresh snapshots after null preparing and empty ready pages", async () => {
    const host = hostPages([
      pageReply({ Preparing: true }),
      pageReply({ SnapshotID: "before-session-birth" }),
      pageReply({ Targets: [target], SnapshotID: "after-session-birth" }),
    ]);

    await expect(usageTarget(host.page, "workspace", "task", "session")).resolves.toEqual(target);
    expect(host.post.mock.calls).toHaveLength(3);
    for (const call of host.post.mock.calls) {
      expect(call[1].data).toEqual({
        workspaceId: "workspace",
        body: { operation: "targets", snapshot_id: "", cursor: "" },
      });
    }
  });

  it("keeps pagination pinned and rejects an unproven matching subject", async () => {
    const host = hostPages([
      pageReply({
        Targets: [{ ...target, SubjectBinding: "unproven", SourceScopeStatus: "unknown" }],
        SnapshotID: "stable-snapshot",
        NextCursor: "next-subject",
        HasMore: true,
      }),
      pageReply({ Targets: [target], SnapshotID: "stable-snapshot" }),
    ]);

    await expect(usageTarget(host.page, "workspace", "task", "session")).resolves.toEqual(target);
    expect(host.post.mock.calls).toHaveLength(2);
    expect(host.post.mock.calls[1]).toEqual([
      expect.any(String),
      {
        data: {
          workspaceId: "workspace",
          body: {
            operation: "targets",
            snapshot_id: "stable-snapshot",
            cursor: "next-subject",
          },
        },
      },
    ]);
  });

  it("fails a denied scope immediately instead of polling it into success", async () => {
    const host = hostPages([pageReply(), pageReply({ Targets: [target] })], 403);
    await expect(usageTarget(host.page, "workspace", "task")).rejects.toThrow("scope denied");
    expect(host.post.mock.calls).toHaveLength(1);
  });

  it("rejects an incomplete page without a continuation cursor", async () => {
    const host = hostPages([pageReply({ Targets: [], HasMore: true })]);
    await expect(usageTarget(host.page, "workspace", "task")).rejects.toThrow();
    expect(host.post.mock.calls).toHaveLength(1);
  });
});

describe("waitForUsage transient read recovery", () => {
  it("continues polling known 503 persistence failures until the exact publication is ready", async () => {
    const host = usagePages([
      usageResponse(503, { error_code: "analytics_busy" }),
      usageResponse(503, { code: "persistence_unavailable" }),
      usageResponse(200, readyUsage),
    ]);

    await expect(waitForUsage(host.page, "workspace", "41")).resolves.toEqual(readyUsage);
    expect(host.get).toHaveBeenCalledTimes(3);
  });

  it("keeps direct reads strict and rejects unknown failures without polling them", async () => {
    const direct = usagePages([usageResponse(503, { error_code: "analytics_busy" })]);
    await expect(readUsage(direct.page, "workspace")).rejects.toThrow("expected 503 to be 200");
    expect(direct.get).toHaveBeenCalledTimes(1);

    const unknown = usagePages([usageResponse(503, { code: "authorization_denied" })]);
    await expect(waitForUsage(unknown.page, "workspace", "41")).rejects.toThrow("received 503");
    expect(unknown.get).toHaveBeenCalledTimes(1);
  });
});
