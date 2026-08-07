import { describe, expect, it, vi } from "vitest";
import type { DockviewApi } from "dockview-react";
import type { TaskPR } from "@/lib/types/github";
import type { TaskMR } from "@/lib/types/gitlab";
import {
  resolveCanonicalReviewPanelState,
  resolveConfiguredReviewPanelPlacement,
  resolveConditionalReviewPanelAction,
  syncCanonicalReviewPanel,
  type CanonicalReviewPanelState,
  type ConditionalReviewPanelOptions,
} from "./dockview-review-panel-sync";

const CENTER_GROUP_ID = "group-center";
const RIGHT_GROUP_ID = "group-right-top";
const SESSION_PANEL_ID = "session:session-a";
const REVIEW_PANEL_ID = "pr-detail";
const PULL_REQUEST_TITLE = "Pull Request";
const githubPRKey = "kandev/kandev/42";
const gitlabMRKey = "https://gitlab.example.test|group/project|7";
const bitbucketReviewKey = "workspace/repository/42";

const DEFAULT_OPTIONS: ConditionalReviewPanelOptions = {
  sessionId: "session-a",
  centerGroupId: CENTER_GROUP_ID,
  reviewsLoaded: true,
  isRestoringLayout: false,
  isMaximized: false,
  wasOffered: false,
};

const githubPR = { owner: "kandev", repo: "kandev", pr_number: 42 } as TaskPR;
const gitlabMR = {
  host: "https://gitlab.example.test",
  project_path: "group/project",
  mr_iid: 7,
} as TaskMR;
const bitbucketReview = {
  providerId: "bitbucket",
  reviewKey: bitbucketReviewKey,
  title: "Bitbucket Pull Request #42",
  url: "https://bitbucket.example/workspace/repository/pull-requests/42",
  repositoryId: "repository-1",
  state: "OPEN",
};

function makeApi(
  panel?: { params?: Record<string, unknown>; groupId?: string; title?: string },
  sessionGroupId = CENTER_GROUP_ID,
  extraGroupIds: string[] = [],
) {
  const updateParameters = vi.fn((next: Record<string, unknown>) => {
    Object.assign(panel?.params ?? {}, next);
  });
  const setTitle = vi.fn();
  const close = vi.fn();
  const addPanel = vi.fn();
  const reviewPanel = panel
    ? {
        id: REVIEW_PANEL_ID,
        params: panel.params ?? {},
        group: { id: panel.groupId ?? RIGHT_GROUP_ID },
        api: { title: panel.title, updateParameters, setTitle, close },
      }
    : undefined;
  const api = {
    getPanel: (id: string) => {
      if (id === REVIEW_PANEL_ID) return reviewPanel;
      if (id === SESSION_PANEL_ID) return { id, group: { id: sessionGroupId } };
      return undefined;
    },
    panels: [
      ...(reviewPanel ? [reviewPanel] : []),
      { id: SESSION_PANEL_ID, group: { id: sessionGroupId } },
    ],
    groups: [
      { id: sessionGroupId },
      ...(sessionGroupId === CENTER_GROUP_ID ? [] : [{ id: CENTER_GROUP_ID }]),
      ...extraGroupIds.map((id) => ({ id })),
    ],
    addPanel,
    removePanel: vi.fn(),
  } as unknown as DockviewApi;
  return { api, updateParameters, setTitle, close, addPanel };
}

describe("resolveCanonicalReviewPanelState", () => {
  it("normalizes GitHub and GitLab identities", () => {
    expect(resolveCanonicalReviewPanelState([githubPR], [])).toEqual({
      params: {
        providerId: "github",
        provider: "github",
        reviewKey: githubPRKey,
        prKey: githubPRKey,
        mrKey: undefined,
      },
      title: PULL_REQUEST_TITLE,
    });
    expect(resolveCanonicalReviewPanelState([], [gitlabMR])).toEqual({
      params: {
        providerId: "gitlab",
        provider: "gitlab",
        reviewKey: gitlabMRKey,
        prKey: undefined,
        mrKey: gitlabMRKey,
      },
      title: "Merge Request",
    });
  });

  it("normalizes a registered provider review", () => {
    expect(resolveCanonicalReviewPanelState([], [], [bitbucketReview])).toEqual({
      params: {
        providerId: "bitbucket",
        provider: undefined,
        reviewKey: bitbucketReviewKey,
        prKey: undefined,
        mrKey: undefined,
      },
      title: "Bitbucket Pull Request #42",
    });
  });

  it("requires selection when providers coexist", () => {
    expect(resolveCanonicalReviewPanelState([githubPR], [gitlabMR], [bitbucketReview])).toEqual({
      params: {
        providerId: undefined,
        provider: undefined,
        reviewKey: undefined,
        prKey: undefined,
        mrKey: undefined,
      },
      title: "Reviews",
    });
  });

  it("returns an empty canonical panel state when no review is linked", () => {
    expect(resolveCanonicalReviewPanelState([], [])).toEqual({
      params: {
        providerId: undefined,
        provider: undefined,
        reviewKey: undefined,
        prKey: undefined,
        mrKey: undefined,
      },
      title: "PR Details",
    });
  });
});

describe("resolveConditionalReviewPanelAction", () => {
  it.each([
    ["add", { hasReview: true, panelExists: false }],
    ["sync", { hasReview: true, panelExists: true }],
    ["remove", { hasReview: false, panelExists: true }],
    ["none", { hasReview: false, panelExists: false }],
  ])("returns %s for the ordinary lifecycle", (expected, input) => {
    expect(
      resolveConditionalReviewPanelAction({
        reviewsLoaded: true,
        isRestoringLayout: false,
        isMaximized: false,
        wasOffered: false,
        ...input,
      }),
    ).toBe(expected);
  });

  it("does not add during restore, maximize, or after dismissal", () => {
    for (const blocked of [
      { isRestoringLayout: true },
      { isMaximized: true },
      { wasOffered: true },
    ]) {
      expect(
        resolveConditionalReviewPanelAction({
          hasReview: true,
          panelExists: false,
          reviewsLoaded: true,
          isRestoringLayout: false,
          isMaximized: false,
          wasOffered: false,
          ...blocked,
        }),
      ).toBe("none");
    }
  });

  it("waits for review hydration before removing the panel", () => {
    expect(
      resolveConditionalReviewPanelAction({
        hasReview: false,
        panelExists: true,
        reviewsLoaded: false,
        isRestoringLayout: false,
        isMaximized: false,
        wasOffered: false,
      }),
    ).toBe("none");
  });
});

describe("resolveConfiguredReviewPanelPlacement", () => {
  it("returns the saved group and tab index", () => {
    expect(
      resolveConfiguredReviewPanelPlacement({
        columns: [
          {
            id: "right",
            groups: [
              {
                id: RIGHT_GROUP_ID,
                panels: [
                  { id: "files", component: "files", title: "Files" },
                  { id: REVIEW_PANEL_ID, component: REVIEW_PANEL_ID, title: "PR Details" },
                ],
              },
            ],
          },
        ],
      }),
    ).toEqual({ groupId: RIGHT_GROUP_ID, index: 1 });
  });
});

describe("syncCanonicalReviewPanel", () => {
  function sync(api: DockviewApi, next: CanonicalReviewPanelState, options = DEFAULT_OPTIONS) {
    return syncCanonicalReviewPanel(api, next, options);
  }

  it("adds a GitHub review beside the live Agent without activating it", () => {
    const { api, addPanel } = makeApi();
    expect(sync(api, resolveCanonicalReviewPanelState([githubPR], []))).toBe(true);
    expect(addPanel).toHaveBeenCalledWith(
      expect.objectContaining({
        id: REVIEW_PANEL_ID,
        title: PULL_REQUEST_TITLE,
        inactive: true,
        position: { referenceGroup: CENTER_GROUP_ID },
        params: expect.objectContaining({ providerId: "github", reviewKey: githubPRKey }),
      }),
    );
  });

  it("adds registered and mixed-provider review surfaces", () => {
    for (const next of [
      resolveCanonicalReviewPanelState([], [], [bitbucketReview]),
      resolveCanonicalReviewPanelState([githubPR], [], [bitbucketReview]),
    ]) {
      const { api, addPanel } = makeApi();
      expect(sync(api, next)).toBe(true);
      expect(addPanel).toHaveBeenCalledWith(
        expect.objectContaining({ id: REVIEW_PANEL_ID, title: next.title, params: next.params }),
      );
    }
  });

  it("honors the saved layout placement", () => {
    const { api, addPanel } = makeApi(undefined, CENTER_GROUP_ID, [RIGHT_GROUP_ID]);
    expect(
      sync(api, resolveCanonicalReviewPanelState([githubPR], []), {
        ...DEFAULT_OPTIONS,
        configuredPlacement: { groupId: RIGHT_GROUP_ID, index: 2 },
      }),
    ).toBe(true);
    expect(addPanel).toHaveBeenCalledWith(
      expect.objectContaining({ position: { referenceGroup: RIGHT_GROUP_ID, index: 2 } }),
    );
  });

  it("closes the panel only after empty review data is loaded", () => {
    const { api, close } = makeApi({ params: { providerId: "github" } });
    expect(
      sync(api, resolveCanonicalReviewPanelState([], []), {
        ...DEFAULT_OPTIONS,
        reviewsLoaded: false,
      }),
    ).toBe(false);
    expect(close).not.toHaveBeenCalled();
    expect(sync(api, resolveCanonicalReviewPanelState([], []))).toBe(true);
    expect(close).toHaveBeenCalledOnce();
  });

  it("updates identity and title without moving the existing panel", () => {
    const params: Record<string, unknown> = { providerId: "gitlab", reviewKey: "old" };
    const { api, updateParameters, setTitle } = makeApi({
      params,
      groupId: "custom-review-group",
      title: "Merge Request",
    });
    expect(sync(api, resolveCanonicalReviewPanelState([githubPR], []))).toBe(true);
    expect(updateParameters).toHaveBeenCalledWith(
      expect.objectContaining({ providerId: "github", reviewKey: githubPRKey }),
    );
    expect(setTitle).toHaveBeenCalledWith(PULL_REQUEST_TITLE);
    expect(api.getPanel(REVIEW_PANEL_ID)?.group.id).toBe("custom-review-group");
  });

  it("updates a registered review title without churning its identity", () => {
    const next = resolveCanonicalReviewPanelState([], [], [bitbucketReview]);
    const { api, updateParameters, setTitle } = makeApi({
      params: { ...next.params },
      title: PULL_REQUEST_TITLE,
    });
    expect(sync(api, next)).toBe(true);
    expect(updateParameters).not.toHaveBeenCalled();
    expect(setTitle).toHaveBeenCalledWith(bitbucketReview.title);
  });
});
