import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { pluginRegistry } from "@/lib/plugins/registry";
import { resolveReviewPanelProvider, useNormalizedTaskReviews } from "./review-panel-provider";

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: Record<string, unknown>) => unknown) =>
    selector({
      workspaces: { activeId: "workspace-a" },
      taskPRs: { byTaskId: {} },
      taskMRs: { byWorkspaceId: {} },
    }),
}));

vi.mock("@/hooks/domains/gitlab/use-task-mr", () => ({ useTaskMRs: () => [] }));

const BITBUCKET_REVIEW_KEY = "workspace/repository/42";
const REVIEW_RELOAD_PLUGIN_ID = "review-reload-plugin";

afterEach(() => {
  cleanup();
  pluginRegistry.unregisterPlugin(REVIEW_RELOAD_PLUGIN_ID);
});

describe("resolveReviewPanelProvider", () => {
  it("routes a GitLab-stamped panel to merge request detail", () => {
    expect(
      resolveReviewPanelProvider({ provider: "gitlab", mrKey: "gitlab.com|a/b|7" }, true, true),
    ).toBe("gitlab");
  });

  it("preserves the legacy GitHub panel when both providers are linked", () => {
    expect(resolveReviewPanelProvider({}, true, true)).toBe("github");
  });

  it("falls back to GitLab when the task has no GitHub pull request", () => {
    expect(resolveReviewPanelProvider({}, false, true)).toBe("gitlab");
  });

  it("keeps a registered provider id instead of treating it as a GitHub or GitLab alias", () => {
    expect(
      String(
        resolveReviewPanelProvider(
          { providerId: "bitbucket", reviewKey: BITBUCKET_REVIEW_KEY },
          false,
          false,
        ),
      ),
    ).toBe("bitbucket");
  });
});

describe("useNormalizedTaskReviews", () => {
  it("does not read or refresh plugin snapshots without an active task", () => {
    const getSnapshot = vi.fn().mockReturnValue([review("Wrong task")]);
    const refresh = vi.fn().mockResolvedValue(undefined);
    pluginRegistry.forPlugin(REVIEW_RELOAD_PLUGIN_ID).registerReviewProvider({
      id: "bitbucket",
      label: "Bitbucket",
      changeRequestNoun: "pull request",
      order: 0,
      getSnapshot,
      subscribe: () => () => {},
      refresh,
      ReviewPanel: () => null,
    });

    const { result } = renderHook(() => useNormalizedTaskReviews(null));

    expect(result.current).toEqual([]);
    expect(getSnapshot).not.toHaveBeenCalled();
    expect(refresh).not.toHaveBeenCalled();
  });

  it("deduplicates concurrent normalized consumers for one provider and task", async () => {
    const refresh = vi.fn(() => new Promise<void>(() => undefined));
    pluginRegistry.forPlugin(REVIEW_RELOAD_PLUGIN_ID).registerReviewProvider({
      id: "bitbucket",
      label: "Bitbucket",
      changeRequestNoun: "pull request",
      order: 0,
      getSnapshot: () => [],
      subscribe: () => () => {},
      refresh,
      ReviewPanel: () => null,
    });

    renderHook(() => {
      useNormalizedTaskReviews("task-a");
      useNormalizedTaskReviews("task-a");
    });

    await waitFor(() => expect(refresh).toHaveBeenCalledOnce());
  });

  it("uses the replacement provider after an unload/reload with the same owner and id", () => {
    const first = {
      id: "bitbucket",
      label: "Bitbucket",
      changeRequestNoun: "pull request",
      order: 0,
      getSnapshot: () => [review("First title")],
      subscribe: () => () => {},
      refresh: async () => {},
      ReviewPanel: () => null,
    };
    const second = { ...first, getSnapshot: () => [review("Second title")] };
    pluginRegistry.forPlugin(REVIEW_RELOAD_PLUGIN_ID).registerReviewProvider(first);

    const { result } = renderHook(() => useNormalizedTaskReviews("task-a"));
    expect(result.current[0]?.title).toBe("First title");

    act(() => {
      pluginRegistry.unregisterPlugin(REVIEW_RELOAD_PLUGIN_ID);
      pluginRegistry.forPlugin(REVIEW_RELOAD_PLUGIN_ID).registerReviewProvider(second);
    });

    expect(result.current[0]?.title).toBe("Second title");
  });
});

function review(title: string) {
  return {
    providerId: "bitbucket",
    reviewKey: BITBUCKET_REVIEW_KEY,
    title,
    url: "https://bitbucket.test/workspace/repository/pull-requests/42",
    repositoryId: "workspace/repository",
    state: "OPEN",
  };
}
