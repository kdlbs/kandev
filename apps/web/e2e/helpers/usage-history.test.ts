import { describe, expect, it, vi } from "vitest";
import type { Page } from "@playwright/test";
import { usageTarget, type UsageTarget } from "./usage-history";

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
