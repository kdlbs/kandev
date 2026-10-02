import type { ChangeEvent } from "react";
import { act, cleanup, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { FileTreeCacheBinding } from "./file-browser-tree-cache";

const search = vi.hoisted(() => vi.fn());
vi.mock("@/lib/ws/connection", () => ({ getWebSocketClient: () => ({}) }));
vi.mock("@/lib/ws/workspace-files", () => ({
  searchWorkspaceFiles: search,
  requestFileTree: vi.fn(),
  requestFileContent: vi.fn(),
}));
import { useFileBrowserSearch } from "./file-browser-hooks";

function deferred() {
  let resolve!: (value: { files: string[] }) => void;
  let reject!: (reason: Error) => void;
  const promise = new Promise<{ files: string[] }>((done, fail) => {
    resolve = done;
    reject = fail;
  });
  return { promise, resolve, reject };
}
const event = (value: string) => ({ target: { value } }) as ChangeEvent<HTMLInputElement>;
function mount() {
  const hook = renderHook(({ id }) => useFileBrowserSearch(id), { initialProps: { id: "A" } });
  act(() => hook.result.current.setIsSearchActive(true));
  return hook;
}
type Hook = Pick<ReturnType<typeof mount>, "result">;
async function query(hook: Hook, value: string) {
  act(() => hook.result.current.handleSearchChange(event(value)));
  await act(() => vi.advanceTimersByTimeAsync(300));
}
beforeEach(() => {
  vi.useFakeTimers();
  search.mockReset();
  vi.spyOn(console, "error").mockImplementation(() => {});
});
afterEach(() => {
  cleanup();
  vi.useRealTimers();
  vi.restoreAllMocks();
});

// @covers AC-UI-TASK-NAVIGATION-RESPONSIVENESS-001.5
// @covers AC-UI-TASK-NAVIGATION-RESPONSIVENESS-001.6
describe("Files search reply ownership", () => {
  it("keeps current results when the previous query completes last", async () => {
    const old = deferred();
    const current = deferred();
    search.mockReturnValueOnce(old.promise).mockReturnValueOnce(current.promise);
    const hook = mount();
    await query(hook, "old");
    await query(hook, "current");
    await act(async () => current.resolve({ files: ["current.ts"] }));
    await act(async () => old.resolve({ files: ["old.ts"] }));
    expect(hook.result.current.searchResults).toEqual(["current.ts"]);
  });

  it("invalidates the old finally as soon as input changes before debounce", async () => {
    const old = deferred();
    search.mockReturnValueOnce(old.promise);
    const hook = mount();
    await query(hook, "old");
    act(() => hook.result.current.handleSearchChange(event("current")));
    await act(async () => old.resolve({ files: ["old.ts"] }));
    expect(hook.result.current.isSearching).toBe(true);
    expect(hook.result.current.searchResults).toBeNull();
  });

  it.each(["clear", "close", "deactivate"])("rejects pending success after %s", async (action) => {
    const old = deferred();
    search.mockReturnValueOnce(old.promise);
    const hook = mount();
    await query(hook, "old");
    act(() => {
      if (action === "clear") hook.result.current.handleSearchChange(event(""));
      else if (action === "close") hook.result.current.handleCloseSearch();
      else hook.result.current.setIsSearchActive(false);
    });
    await act(async () => old.resolve({ files: ["old.ts"] }));
    expect(hook.result.current.searchResults).toBeNull();
    expect(hook.result.current.isSearching).toBe(false);
  });

  it("rejects stale errors and finally while the replacement is pending", async () => {
    const old = deferred();
    const current = deferred();
    search.mockReturnValueOnce(old.promise).mockReturnValueOnce(current.promise);
    const hook = mount();
    await query(hook, "old");
    await query(hook, "current");
    await act(async () => old.reject(new Error("obsolete failure")));
    expect(hook.result.current.searchResults).toBeNull();
    expect(hook.result.current.isSearching).toBe(true);
    expect(console.error).not.toHaveBeenCalled();
    await act(async () => current.resolve({ files: ["current.ts"] }));
    expect(hook.result.current.isSearching).toBe(false);
  });

  it("settles the current error", async () => {
    const current = deferred();
    search.mockReturnValueOnce(current.promise);
    const hook = mount();
    await query(hook, "current");
    await act(async () => current.reject(new Error("current failure")));
    expect(hook.result.current.searchResults).toEqual([]);
    expect(hook.result.current.isSearching).toBe(false);
    expect(console.error).toHaveBeenCalledTimes(1);
  });
});

// @covers AC-UI-TASK-NAVIGATION-RESPONSIVENESS-001.5
describe("Files search context retirement", () => {
  it.each([false, true])("retires prior session data, including return=%s", async (returnToA) => {
    const old = deferred();
    search.mockReturnValueOnce(old.promise);
    const hook = mount();
    await query(hook, "old");
    hook.rerender({ id: "B" });
    if (returnToA) hook.rerender({ id: "A" });
    await act(async () => old.resolve({ files: ["A-only.ts"] }));
    expect(hook.result.current.searchResults).toBeNull();
    expect(hook.result.current.localSearchQuery).toBe("");
    expect(hook.result.current.isSearching).toBe(false);
  });

  it("clears a settled previous-session snapshot synchronously", async () => {
    search.mockResolvedValue({ files: ["A-only.ts"] });
    const hook = mount();
    await query(hook, "old");
    hook.rerender({ id: "B" });
    expect(hook.result.current.searchResults).toBeNull();
  });

  it.each(["switch", "close", "unmount"])(
    "cancels an undispatched debounce on %s",
    async (action) => {
      const hook = mount();
      act(() => hook.result.current.handleSearchChange(event("old")));
      if (action === "switch") hook.rerender({ id: "B" });
      else if (action === "close") act(() => hook.result.current.handleCloseSearch());
      else hook.unmount();
      await act(() => vi.advanceTimersByTimeAsync(300));
      expect(search).not.toHaveBeenCalled();
    },
  );

  it("silences a pending error after unmount", async () => {
    const old = deferred();
    search.mockReturnValueOnce(old.promise);
    const hook = mount();
    await query(hook, "old");
    hook.unmount();
    await act(async () => old.reject(new Error("retired failure")));
    expect(console.error).not.toHaveBeenCalled();
  });

  it("rejects results after the binding retires without a rerender", async () => {
    let current = true;
    const binding = { isCurrent: () => current } as FileTreeCacheBinding;
    const hook = renderHook(() => useFileBrowserSearch("A", binding));
    act(() => hook.result.current.setIsSearchActive(true));
    const old = deferred();
    search.mockReturnValueOnce(old.promise);
    await query(hook, "old");
    current = false;
    await act(async () => old.resolve({ files: ["old.ts"] }));
    expect(hook.result.current.searchResults).toBeNull();
  });
});

// @covers AC-UI-TASK-NAVIGATION-RESPONSIVENESS-001.5
it.each(["binding", "reset"])("retires the same session's search on %s change", async (change) => {
  const initialBinding = { isCurrent: () => true } as FileTreeCacheBinding;
  const hook = renderHook(({ binding, reset }) => useFileBrowserSearch("A", binding, reset), {
    initialProps: { binding: initialBinding, reset: "initial" },
  });
  act(() => hook.result.current.setIsSearchActive(true));
  const pending = deferred();
  search.mockReturnValueOnce(pending.promise);
  await query(hook, "old");
  hook.rerender({
    binding: change === "binding" ? { ...initialBinding } : initialBinding,
    reset: change === "reset" ? "replacement" : "initial",
  });
  await act(async () => pending.resolve({ files: ["old.ts"] }));
  expect(hook.result.current.searchResults).toBeNull();
  expect(hook.result.current.isSearching).toBe(false);
});
