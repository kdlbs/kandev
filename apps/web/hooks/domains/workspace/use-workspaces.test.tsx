import { describe, expect, it } from "vitest";
import { renderHook, waitFor } from "@testing-library/react";
import { QueryClientProvider } from "@tanstack/react-query";
import { type ReactNode } from "react";
import { StateProvider, useAppStore } from "@/components/state-provider";
import { makeQueryClient } from "@/lib/query/client";
import { qk } from "@/lib/query/keys";
import type { Workspace } from "@/lib/types/http";
import { useWorkspaces } from "./use-workspaces";

function workspace(id: string, name = id): Workspace {
  return {
    id,
    name,
    owner_id: "user-1",
    created_at: "2026-01-01T00:00:00Z",
    updated_at: "2026-01-01T00:00:00Z",
  } as Workspace;
}

function renderUseWorkspaces({
  activeId = null,
  activeWorkflowId = null,
  workspaces = [],
}: {
  activeId?: string | null;
  activeWorkflowId?: string | null;
  workspaces?: Workspace[];
} = {}) {
  const queryClient = makeQueryClient();
  queryClient.setQueryData(qk.workspaces.all(), workspaces);
  const wrapper = ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={queryClient}>
      <StateProvider
        initialState={{ workspaces: { activeId }, workflows: { activeId: activeWorkflowId } }}
      >
        {children}
      </StateProvider>
    </QueryClientProvider>
  );

  return { ...renderHook(() => useWorkspaces(), { wrapper }), queryClient };
}

describe("useWorkspaces", () => {
  it("reads the workspace list from the query cache", () => {
    const { result } = renderUseWorkspaces({
      activeId: "ws-1",
      workspaces: [workspace("ws-1", "Main")],
    });

    expect(result.current.items).toEqual([expect.objectContaining({ id: "ws-1", name: "Main" })]);
    expect(result.current.activeWorkspace?.name).toBe("Main");
  });

  it("repairs a missing active workspace id from the query list", async () => {
    const { result } = renderUseWorkspaces({
      activeId: "missing",
      activeWorkflowId: "wf-1",
      workspaces: [workspace("ws-2", "Next")],
    });

    await waitFor(() => expect(result.current.activeId).toBe("ws-2"));
    expect(result.current.activeWorkspace?.name).toBe("Next");
  });

  it("clears active workspace and workflow when the query list is empty", async () => {
    const { result } = renderHook(
      () => ({
        workspaces: useWorkspaces(),
        activeWorkflowId: useAppStore((state) => state.workflows.activeId),
      }),
      {
        wrapper: ({ children }) => {
          const queryClient = makeQueryClient();
          queryClient.setQueryData(qk.workspaces.all(), []);
          return (
            <QueryClientProvider client={queryClient}>
              <StateProvider
                initialState={{
                  workspaces: { activeId: "deleted" },
                  workflows: { activeId: "wf-1" },
                }}
              >
                {children}
              </StateProvider>
            </QueryClientProvider>
          );
        },
      },
    );

    await waitFor(() => expect(result.current.workspaces.activeId).toBeNull());
    expect(result.current.activeWorkflowId).toBeNull();
  });
});
