import { describe, expect, it, vi } from "vitest";
import type { DockviewApi } from "dockview-react";
import type { TaskPR } from "@/lib/types/github";
import type { TaskMR } from "@/lib/types/gitlab";
import {
  resolveCanonicalReviewParams,
  resolveConditionalReviewPanelAction,
  syncCanonicalReviewPanel,
} from "./dockview-review-panel-sync";

const CENTER_GROUP_ID = "group-center";
const SESSION_PANEL_ID = "session:session-a";
const PR_KEY = "kandev/kandev/42";

const syncWithOptions = syncCanonicalReviewPanel as unknown as (
  api: DockviewApi,
  next: ReturnType<typeof resolveCanonicalReviewParams>,
  options: {
    sessionId: string;
    centerGroupId: string;
    isRestoringLayout: boolean;
    isMaximized: boolean;
    wasOffered: boolean;
  },
) => boolean;

function makeApi(
  panel?: { params?: Record<string, unknown>; groupId?: string },
  sessionGroupId = CENTER_GROUP_ID,
): {
  api: DockviewApi;
  updateParameters: ReturnType<typeof vi.fn>;
  close: ReturnType<typeof vi.fn>;
  addPanel: ReturnType<typeof vi.fn>;
} {
  const updateParameters = vi.fn((next: Record<string, unknown>) => {
    Object.assign(panel?.params ?? {}, next);
  });
  const close = vi.fn();
  const addPanel = vi.fn();
  const reviewPanel = panel
    ? {
        id: "pr-detail",
        params: panel.params ?? {},
        group: { id: panel.groupId ?? "group-right-top" },
        api: { updateParameters, close },
      }
    : undefined;
  return {
    api: {
      getPanel: (id: string) => {
        if (id === "pr-detail") return reviewPanel;
        if (id === SESSION_PANEL_ID) return { id, group: { id: sessionGroupId } };
        return undefined;
      },
      panels: [
        ...(reviewPanel ? [reviewPanel] : []),
        { id: SESSION_PANEL_ID, group: { id: sessionGroupId } },
      ],
      groups: [{ id: sessionGroupId }],
      addPanel,
      removePanel: vi.fn(),
    } as unknown as DockviewApi,
    updateParameters,
    close,
    addPanel,
  };
}

const githubPR = {
  owner: "kandev",
  repo: "kandev",
  pr_number: 42,
} as TaskPR;

const gitlabMR = {
  host: "https://gitlab.example.test",
  project_path: "group/project",
  mr_iid: 7,
} as TaskMR;

describe("resolveCanonicalReviewParams", () => {
  it("prefers the primary GitHub pull request when both providers are linked", () => {
    expect(resolveCanonicalReviewParams([githubPR], [gitlabMR])).toEqual({
      provider: "github",
      prKey: PR_KEY,
      mrKey: undefined,
    });
  });

  it("selects the first linked GitLab merge request when GitHub is absent", () => {
    expect(resolveCanonicalReviewParams([], [gitlabMR])).toEqual({
      provider: "gitlab",
      prKey: undefined,
      mrKey: "https://gitlab.example.test|group/project|7",
    });
  });

  it("clears review identity when the active task has no linked review", () => {
    expect(resolveCanonicalReviewParams([], [])).toEqual({
      provider: undefined,
      prKey: undefined,
      mrKey: undefined,
    });
  });
});

describe("resolveConditionalReviewPanelAction", () => {
  it.each([
    [
      "adds a linked review",
      { hasReview: true, panelExists: false, autoAddedForReview: false, wasOffered: false },
      "add",
    ],
    [
      "waits while restoring",
      { hasReview: true, panelExists: false, autoAddedForReview: false, isRestoringLayout: true },
      "none",
    ],
    [
      "waits while maximized",
      { hasReview: true, panelExists: false, autoAddedForReview: false, isMaximized: true },
      "none",
    ],
    [
      "respects a dismissed offer",
      { hasReview: true, panelExists: false, autoAddedForReview: false, wasOffered: true },
      "none",
    ],
    [
      "removes a conditional panel without a review",
      { hasReview: false, panelExists: true, autoAddedForReview: true },
      "remove",
    ],
    [
      "syncs an explicit panel without a review",
      { hasReview: false, panelExists: true, autoAddedForReview: false },
      "sync",
    ],
  ])("$0", (_name, input, expected) => {
    expect(
      resolveConditionalReviewPanelAction({
        isRestoringLayout: false,
        isMaximized: false,
        wasOffered: false,
        ...input,
      }),
    ).toBe(expected);
  });
});

describe("syncCanonicalReviewPanel", () => {
  it("leaves a layout without PR Details structurally untouched", () => {
    const { api, updateParameters, addPanel } = makeApi();

    expect(syncCanonicalReviewPanel(api, resolveCanonicalReviewParams([githubPR], []))).toBe(false);
    expect(updateParameters).not.toHaveBeenCalled();
    expect(addPanel).not.toHaveBeenCalled();
    expect(api.removePanel).not.toHaveBeenCalled();
  });

  it("adds a linked review beside the live Agent without activating it", () => {
    const { api, addPanel } = makeApi();
    expect(
      syncWithOptions(api, resolveCanonicalReviewParams([githubPR], []), {
        sessionId: "session-a",
        centerGroupId: CENTER_GROUP_ID,
        isRestoringLayout: false,
        isMaximized: false,
        wasOffered: false,
      }),
    ).toBe(true);
    expect(addPanel).toHaveBeenCalledWith(
      expect.objectContaining({
        id: "pr-detail",
        component: "pr-detail",
        inactive: true,
        position: { referenceGroup: CENTER_GROUP_ID },
        params: {
          provider: "github",
          prKey: PR_KEY,
          mrKey: undefined,
          autoAddedForReview: true,
        },
      }),
    );
  });

  it("does not add while restoring, maximized, or dismissed", () => {
    for (const options of [
      { isRestoringLayout: true, isMaximized: false, wasOffered: false },
      { isRestoringLayout: false, isMaximized: true, wasOffered: false },
      { isRestoringLayout: false, isMaximized: false, wasOffered: true },
    ]) {
      const { api, addPanel } = makeApi();
      expect(
        syncWithOptions(api, resolveCanonicalReviewParams([githubPR], []), {
          sessionId: "session-a",
          centerGroupId: CENTER_GROUP_ID,
          ...options,
        }),
      ).toBe(false);
      expect(addPanel).not.toHaveBeenCalled();
    }
  });

  it("closes a conditionally added panel when review data disappears", () => {
    const { api, close } = makeApi({ params: { autoAddedForReview: true } });

    expect(syncCanonicalReviewPanel(api, resolveCanonicalReviewParams([], []))).toBe(true);
    expect(close).toHaveBeenCalledOnce();
  });

  it("preserves an explicitly configured panel when review data disappears", () => {
    const { api, close, updateParameters } = makeApi({ params: { provider: "github" } });

    expect(syncCanonicalReviewPanel(api, resolveCanonicalReviewParams([], []))).toBe(true);
    expect(close).not.toHaveBeenCalled();
    expect(updateParameters).toHaveBeenCalledWith({
      provider: undefined,
      prKey: undefined,
      mrKey: undefined,
    });
  });

  it("updates an existing panel's identity without changing its configured group", () => {
    const params: Record<string, unknown> = { provider: "gitlab", mrKey: "old/mr" };
    const { api, updateParameters } = makeApi({ params, groupId: "custom-review-group" });

    expect(syncCanonicalReviewPanel(api, resolveCanonicalReviewParams([githubPR], []))).toBe(true);
    expect(updateParameters).toHaveBeenCalledWith({
      provider: "github",
      prKey: PR_KEY,
      mrKey: undefined,
    });
    expect(api.getPanel("pr-detail")?.group.id).toBe("custom-review-group");
    expect(params).toMatchObject({ provider: "github", prKey: PR_KEY });
    expect(params.mrKey).toBeUndefined();
  });
});
