import { describe, expect, it, vi } from "vitest";
import type { DockviewApi } from "dockview-react";
import type { TaskPR } from "@/lib/types/github";
import type { TaskMR } from "@/lib/types/gitlab";
import {
  resolveCanonicalReviewParams,
  syncCanonicalReviewPanel,
} from "./dockview-review-panel-sync";

function makeApi(panel?: { params?: Record<string, unknown>; groupId?: string }): {
  api: DockviewApi;
  updateParameters: ReturnType<typeof vi.fn>;
} {
  const updateParameters = vi.fn((next: Record<string, unknown>) => {
    Object.assign(panel?.params ?? {}, next);
  });
  const reviewPanel = panel
    ? {
        id: "pr-detail",
        params: panel.params ?? {},
        group: { id: panel.groupId ?? "group-right-top" },
        api: { updateParameters },
      }
    : undefined;
  return {
    api: {
      getPanel: (id: string) => (id === "pr-detail" ? reviewPanel : undefined),
      addPanel: vi.fn(),
      removePanel: vi.fn(),
    } as unknown as DockviewApi,
    updateParameters,
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
      prKey: "kandev/kandev/42",
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

describe("syncCanonicalReviewPanel", () => {
  it("leaves a layout without PR Details structurally untouched", () => {
    const { api, updateParameters } = makeApi();

    expect(syncCanonicalReviewPanel(api, resolveCanonicalReviewParams([githubPR], []))).toBe(false);
    expect(updateParameters).not.toHaveBeenCalled();
    expect(api.addPanel).not.toHaveBeenCalled();
    expect(api.removePanel).not.toHaveBeenCalled();
  });

  it("updates an existing panel's identity without changing its configured group", () => {
    const params: Record<string, unknown> = { provider: "gitlab", mrKey: "old/mr" };
    const { api, updateParameters } = makeApi({ params, groupId: "custom-review-group" });

    expect(syncCanonicalReviewPanel(api, resolveCanonicalReviewParams([githubPR], []))).toBe(true);
    expect(updateParameters).toHaveBeenCalledWith({
      provider: "github",
      prKey: "kandev/kandev/42",
      mrKey: undefined,
    });
    expect(api.getPanel("pr-detail")?.group.id).toBe("custom-review-group");
    expect(params).toMatchObject({ provider: "github", prKey: "kandev/kandev/42" });
    expect(params.mrKey).toBeUndefined();
  });
});
