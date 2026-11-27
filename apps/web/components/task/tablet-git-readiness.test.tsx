import type { ReactNode } from "react";
import { act, cleanup, renderHook } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import type { GitStatusEntry } from "@/lib/state/slices/session-runtime/types";
import { useCenterPanelGitChanges } from "./task-center-panel-state";
const client = vi.hoisted(() => ({ request: vi.fn(async () => ({ commits: [], ready: true })) }));
vi.mock("@/lib/ws/connection", async (original) => ({
  ...(await original<object>()),
  getWebSocketClient: () => client,
}));
afterEach(cleanup);
function status(repository: string, dirty = false): GitStatusEntry {
  return {
    repository_name: repository,
    status_state: "ready",
    files_complete: true,
    detail_state: "ready",
    branch: "main",
    remote_branch: null,
    modified: dirty ? ["F.txt"] : [],
    added: [],
    deleted: [],
    untracked: [],
    renamed: [],
    ahead: 0,
    behind: 0,
    files: dirty ? { "F.txt": { path: "F.txt", status: "modified", staged: false } } : {},
    timestamp: null,
  };
}
function setup(inventory: string[] | null = ["A", "B"]) {
  const wrapper = ({ children }: { children: ReactNode }) => (
    <StateProvider>{children}</StateProvider>
  );
  const hook = renderHook(
    () => ({ store: useAppStoreApi(), changes: useCenterPanelGitChanges("B") }),
    { wrapper },
  );
  act(() => {
    hook.result.current.store.getState().registerSessionEnvironment("B", "env");
    hook.result.current.store.getState().setSessionCommits("B", [], { allowEmpty: true });
    if (inventory)
      hook.result.current.store.getState().reconcileGitStatusInventory("env", 0, inventory);
  });
  return hook;
}
it.each([false, true])(
  "does not treat a clean mirror as empty workspace (cleanFirst=%s)",
  (cleanFirst) => {
    const hook = setup();
    const update = (repo: string, dirty = false) =>
      act(() => {
        hook.result.current.store.getState().setGitStatus("env", status(repo, dirty));
      });
    if (cleanFirst) {
      update("B");
      expect(hook.result.current.changes).toBeUndefined();
      update("A", true);
    } else {
      update("A", true);
      update("B");
    }
    expect(hook.result.current.changes).toBe(true);
  },
);
it("requires complete current scopes and refresh evidence before declaring ready-empty", () => {
  const hook = setup();
  const state = hook.result.current.store.getState();
  act(() => state.setGitStatus("env", status("B")));
  expect(hook.result.current.changes).toBeUndefined();
  act(() => state.setGitStatus("env", status("A")));
  expect(hook.result.current.changes).toBe(false);
  act(() => state.setGitStatusRefresh("env", "A", { state: "pending" }));
  expect(hook.result.current.changes).toBeUndefined();
  act(() => state.setGitStatusRefresh("env", "A", null));
  expect(hook.result.current.changes).toBe(false);
  act(() => state.invalidateGitStatusInventory("B"));
  expect(hook.result.current.changes).toBeUndefined();
});
it("settles a single unnamed repository without a refreshed inventory", () => {
  const hook = setup(null);
  const state = hook.result.current.store.getState();
  expect(hook.result.current.changes).toBeUndefined();
  act(() => state.setGitStatus("env", { ...status(""), repository_name: undefined }));
  expect(hook.result.current.changes).toBe(false);
  act(() => state.setGitStatus("env", { ...status("", true), repository_name: undefined }));
  expect(hook.result.current.changes).toBe(true);
});
it("keeps named repositories unknown until inventory is validated", () => {
  const hook = setup(null);
  act(() => hook.result.current.store.getState().setGitStatus("env", status("A")));
  expect(hook.result.current.changes).toBeUndefined();
});
it.each(["unavailable", "pending"] as const)(
  "keeps a lone unnamed root unknown while a named scope is %s",
  (state) => {
    const hook = setup(null);
    const store = hook.result.current.store.getState();
    act(() => {
      store.setGitStatus("env", { ...status(""), repository_name: undefined });
      store.setGitStatusRefresh("env", "vendor/lib", { state });
    });
    expect(hook.result.current.changes).toBeUndefined();
  },
);
it.each([
  [1, false],
  [2, undefined],
] as const)(
  "uses the task's %s registered repositories before inferring a lone root",
  (count, expected) => {
    const hook = setup(null);
    const store = hook.result.current.store;
    act(() => {
      store.setState((state) => {
        state.taskSessions.items.B = { id: "B", task_id: "T" } as never;
        state.kanban.tasks = [
          { id: "T", repositories: Array.from({ length: count }, (_, i) => ({ id: `r${i}` })) },
        ] as never;
        return state;
      });
      store.getState().setGitStatus("env", { ...status(""), repository_name: undefined });
    });
    expect(hook.result.current.changes).toBe(expected);
  },
);
it.each([false, true])(
  "reads repositories from the task page's overview before stale kanban rows (stale=%s)",
  (stale) => {
    const hook = setup(null);
    const store = hook.result.current.store;
    const repositories = (count: number) =>
      Array.from({ length: count }, (_, i) => ({ id: `r${i}` }));
    act(() => {
      store.setState((state) => {
        state.tasks.activeTaskId = "T";
        state.taskSessions.items.B = { id: "B", task_id: "T" } as never;
        state.kanban.workflowId = "X";
        state.kanban.tasks = (
          stale ? [{ id: "T", workflowId: "Y", repositories: repositories(1) }] : []
        ) as never;
        return state;
      });
      const readId = store.getState().beginTaskOverviewRead();
      store
        .getState()
        .retainTaskOverviews(
          "detail",
          [{ id: "T", workflowId: "Y", repositories: repositories(2) } as never],
          readId,
        );
      store.getState().setGitStatus("env", { ...status(""), repository_name: undefined });
    });
    expect(store.getState().taskOverview.byId.T?.repositories).toHaveLength(2);
    expect(hook.result.current.changes).toBeUndefined();
  },
);
