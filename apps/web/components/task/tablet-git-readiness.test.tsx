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
function setup() {
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
    hook.result.current.store.getState().reconcileGitStatusInventory("env", 0, ["A", "B"]);
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
