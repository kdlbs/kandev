import { describe, expect, it, vi } from "vitest";
import type { DockviewApi } from "dockview-react";
import type { TaskPR } from "@/lib/types/github";
import type { TaskMR } from "@/lib/types/gitlab";
import {
  resolveCanonicalReviewPanelState,
  syncCanonicalReviewPanel,
} from "./dockview-review-panel-sync";

function makeApi(panel?: { params?: Record<string, unknown>; groupId?: string; title?: string }): {
  api: DockviewApi;
  updateParameters: ReturnType<typeof vi.fn>;
  setTitle: ReturnType<typeof vi.fn>;
} {
  const updateParameters = vi.fn((next: Record<string, unknown>) => {
    Object.assign(panel?.params ?? {}, next);
  });
  const setTitle = vi.fn();
  const reviewPanel = panel
    ? {
        id: "pr-detail",
        params: panel.params ?? {},
        group: { id: panel.groupId ?? "group-right-top" },
        api: { title: panel.title, updateParameters, setTitle },
      }
    : undefined;
  return {
    api: {
      getPanel: (id: string) => (id === "pr-detail" ? reviewPanel : undefined),
      addPanel: vi.fn(),
      removePanel: vi.fn(),
    } as unknown as DockviewApi,
    updateParameters,
    setTitle,
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
const githubPRKey = "kandev/kandev/42";
const gitlabMRKey = "https://gitlab.example.test|group/project|7";
const bitbucketReviewKey = "workspace/repository/42";

describe("resolveCanonicalReviewPanelState", () => {
  it("requires selection when GitHub and GitLab reviews are both linked", () => {
    expect(resolveCanonicalReviewPanelState([githubPR], [gitlabMR])).toEqual({
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

  it("selects the first linked GitLab merge request when GitHub is absent", () => {
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

  it("clears review identity when the active task has no linked review", () => {
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

  it("selects a registered review when no built-in review is linked", () => {
    expect(
      resolveCanonicalReviewPanelState(
        [],
        [],
        [
          {
            providerId: "bitbucket",
            reviewKey: bitbucketReviewKey,
            title: "Bitbucket pull request",
            url: "https://bitbucket.example/workspace/repository/pull-requests/42",
            repositoryId: "repository-1",
            state: "OPEN",
          },
        ],
      ),
    ).toEqual({
      params: {
        providerId: "bitbucket",
        provider: undefined,
        reviewKey: bitbucketReviewKey,
        prKey: undefined,
        mrKey: undefined,
      },
      title: "Bitbucket pull request",
    });
  });

  it("requires selection when built-in and registered providers coexist", () => {
    expect(
      resolveCanonicalReviewPanelState(
        [githubPR],
        [],
        [
          {
            providerId: "bitbucket",
            reviewKey: bitbucketReviewKey,
            title: "Bitbucket pull request",
            url: "https://bitbucket.example/workspace/repository/pull-requests/42",
            repositoryId: "repository-1",
            state: "OPEN",
          },
        ],
      ),
    ).toEqual({
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
});

describe("syncCanonicalReviewPanel", () => {
  it("leaves a layout without PR Details structurally untouched", () => {
    const { api, updateParameters } = makeApi();

    expect(syncCanonicalReviewPanel(api, resolveCanonicalReviewPanelState([githubPR], []))).toBe(
      false,
    );
    expect(updateParameters).not.toHaveBeenCalled();
    expect(api.addPanel).not.toHaveBeenCalled();
    expect(api.removePanel).not.toHaveBeenCalled();
  });

  it("updates an existing panel's identity without changing its configured group", () => {
    const params: Record<string, unknown> = { provider: "gitlab", mrKey: "old/mr" };
    const { api, updateParameters, setTitle } = makeApi({
      params,
      groupId: "custom-review-group",
      title: "Merge Request",
    });

    expect(syncCanonicalReviewPanel(api, resolveCanonicalReviewPanelState([githubPR], []))).toBe(
      true,
    );
    expect(updateParameters).toHaveBeenCalledWith({
      providerId: "github",
      provider: "github",
      reviewKey: githubPRKey,
      prKey: githubPRKey,
      mrKey: undefined,
    });
    expect(setTitle).toHaveBeenCalledWith("Pull Request");
    expect(api.getPanel("pr-detail")?.group.id).toBe("custom-review-group");
    expect(params).toMatchObject({ provider: "github", prKey: githubPRKey });
    expect(params.mrKey).toBeUndefined();
  });

  it("updates a registered review title even when its identity is already current", () => {
    const params: Record<string, unknown> = {
      providerId: "bitbucket",
      provider: undefined,
      reviewKey: bitbucketReviewKey,
      prKey: undefined,
      mrKey: undefined,
    };
    const { api, updateParameters, setTitle } = makeApi({ params, title: "Pull Request" });
    const next = resolveCanonicalReviewPanelState(
      [],
      [],
      [
        {
          providerId: "bitbucket",
          reviewKey: bitbucketReviewKey,
          title: "Bitbucket Pull Request #42",
          url: "https://bitbucket.example/workspace/repository/pull-requests/42",
          repositoryId: "repository-1",
          state: "OPEN",
        },
      ],
    );

    expect(syncCanonicalReviewPanel(api, next)).toBe(true);
    expect(updateParameters).not.toHaveBeenCalled();
    expect(setTitle).toHaveBeenCalledWith("Bitbucket Pull Request #42");
  });
});
