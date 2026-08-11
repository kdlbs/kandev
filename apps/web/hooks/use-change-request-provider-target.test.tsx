import { createElement, type ReactNode } from "react";
import { cleanup, renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { StateProvider } from "@/components/state-provider";
import { pluginRegistry } from "@/lib/plugins/registry";
import type { RepositoryProviderRegistration } from "@/lib/plugins/types";
import type { HydrationState } from "@/lib/state/store";
import { useChangeRequestProviderTarget } from "./use-change-request-provider-target";

const PLUGIN_ID = "multi-workflow-provider-plugin";

afterEach(() => {
  cleanup();
  pluginRegistry.unregisterPlugin(PLUGIN_ID);
});

function wrapper({ children }: { children: ReactNode }) {
  return createElement(StateProvider, null, children);
}

function wrapperWithInitialState(initialState: HydrationState) {
  return function InitialStateWrapper({ children }: { children: ReactNode }) {
    return createElement(StateProvider, { initialState, children });
  };
}

describe("useChangeRequestProviderTarget", () => {
  it("stays render-stable when the workspace has no repositories", () => {
    let renders = 0;
    const { result, rerender } = renderHook(
      () => {
        renders += 1;
        return useChangeRequestProviderTarget(null);
      },
      { wrapper },
    );

    expect(result.current).toBeNull();
    rerender();
    expect(result.current).toBeNull();
    expect(renders).toBeLessThan(5);
  });

  it("resolves a provider target for a task loaded only in a cross-workflow snapshot", () => {
    const provider: RepositoryProviderRegistration = {
      id: "bitbucket",
      label: "Bitbucket",
      listRepositories: async () => [],
      matchesURL: () => false,
      listBranches: async () => [],
      inspectURL: async () => null,
      createChangeRequest: async () => ({ url: "https://bitbucket.example/pull-requests/1" }),
    };
    pluginRegistry.setDeclaredRepositoryProviderIds(PLUGIN_ID, [provider.id]);
    pluginRegistry.forPlugin(PLUGIN_ID).registerRepositoryProvider(provider);
    const initialState: HydrationState = {
      taskSessions: { items: { "session-a": { task_id: "task-a" } as never } },
      kanbanMulti: {
        isLoading: false,
        snapshots: {
          "workflow-b": {
            tasks: [
              {
                id: "task-a",
                workspaceId: "workspace-a",
                repositoryId: "repository-a",
              } as never,
            ],
          } as never,
        },
      },
      repositories: {
        loadingByWorkspaceId: {},
        loadedByWorkspaceId: { "workspace-a": true },
        itemsByWorkspaceId: {
          "workspace-a": [
            {
              id: "repository-a",
              workspace_id: "workspace-a",
              name: "api",
              provider: "bitbucket",
            } as never,
          ],
        },
      },
    };

    const { result } = renderHook(() => useChangeRequestProviderTarget("session-a"), {
      wrapper: wrapperWithInitialState(initialState),
    });

    expect(result.current?.taskId).toBe("task-a");
  });
});
