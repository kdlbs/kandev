import { useState, type ReactNode } from "react";
import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import { useTabletSessionTab } from "@/hooks/domains/session/use-tablet-session-tab";
import { useTaskCenterFileOpen } from "@/hooks/use-task-center-file-open";
import { useTabletFileState } from "./use-tablet-file-state";
import { getFileTabKey, upsertOpenFileTab } from "./task-center-panel-file-tabs";
import { getOpenFileTabs, setActiveTabForSession, setOpenFileTabs } from "@/lib/local-storage";
import type { OpenFileTab } from "@/lib/types/backend";
const read = vi.hoisted(() => vi.fn());
vi.mock("@/lib/ws/connection", () => ({ getWebSocketClient: () => ({}) }));
vi.mock("@/lib/ws/workspace-files", () => ({
  requestFileContent: (...args: unknown[]) => read(...args),
}));
afterEach(() => {
  cleanup();
  sessionStorage.clear();
  read.mockReset();
});
const f: OpenFileTab = {
  path: "F.txt",
  name: "F.txt",
  content: "F",
  originalContent: "F",
  originalHash: "hash",
  isDirty: false,
};
function setup() {
  const wrapper = ({ children }: { children: ReactNode }) => (
    <StateProvider>{children}</StateProvider>
  );
  return renderHook(
    ({ session }) => {
      const store = useAppStoreApi();
      const [tab, select] = useTabletSessionTab(session);
      const [tabs, setTabs] = useState<OpenFileTab[]>([]);
      const files = useTabletFileState({
        adapter: undefined,
        session,
        tabs,
        setTabs,
        selected: tab,
        setSelected: select,
      });
      const open = useTaskCenterFileOpen({
        activeSessionId: session,
        addFileTab: (file) => {
          setTabs((prev) => upsertOpenFileTab(prev, file));
          select(`file:${getFileTabKey(file)}`);
        },
        toast: vi.fn(),
      });
      return { store, tab, select, tabs, setTabs, open, retireFile: files.retireFile };
    },
    { wrapper, initialProps: { session: "B" } },
  );
}
it.each([false, true])(
  "falls back from an unavailable initial file (descriptor=%s)",
  async (descriptor) => {
    setActiveTabForSession("B", "file:F.txt");
    if (descriptor)
      setOpenFileTabs("B", [
        { path: "F.txt", name: "F.txt" },
        { path: "G.txt", name: "G.txt" },
      ]);
    read.mockImplementation((_client, _id, path) =>
      path === "F.txt" ? Promise.reject(new Error("not found")) : Promise.resolve({ content: "G" }),
    );
    const hook = setup();
    await waitFor(() => expect(hook.result.current.tab).toBe("chat"));
  },
);
it.each([false, true])(
  "does not resurrect a closed file or saved descriptor (alias=%s)",
  async (alias) => {
    setActiveTabForSession("B", "file:F.txt");
    setOpenFileTabs("B", [
      { path: "F.txt", name: "F.txt", ...(alias ? { repo: "repo" } : {}) },
      { path: "G.txt", name: "G.txt" },
    ]);
    const pending = Promise.withResolvers<{ content: string; resolved_path?: string }>();
    read.mockImplementation((_client, _id, path) =>
      path === "F.txt" ? pending.promise : Promise.resolve({ content: "G" }),
    );
    const hook = setup();
    const live = alias ? { ...f, path: "repo/F.txt", resolvedPath: "repo/F.txt" } : f;
    act(() => hook.result.current.setTabs([live]));
    act(() => {
      hook.result.current.retireFile(live);
      hook.result.current.setTabs([]);
      hook.result.current.select("chat");
    });
    await act(async () =>
      pending.resolve({ content: "old F", ...(alias ? { resolved_path: "repo/F.txt" } : {}) }),
    );
    await waitFor(() =>
      expect(hook.result.current.tabs.map((file) => file.path)).toEqual(["G.txt"]),
    );
    await waitFor(() => expect(getOpenFileTabs("B").map((file) => file.path)).toEqual(["G.txt"]));
  },
);
it("does not let an older tool open replace a newer explicit Chat admission", async () => {
  const hook = setup();
  const pending = Promise.withResolvers<{ content: string }>();
  read.mockReturnValue(pending.promise);
  let opening!: Promise<void>;
  act(() => {
    opening = hook.result.current.open("F.txt");
  });
  act(() => hook.result.current.select("chat"));
  await act(async () => {
    pending.resolve({ content: "old F" });
    await opening;
  });
  expect(hook.result.current.tab).toBe("chat");
});

it("restores the same file independently when changing tablet sessions", async () => {
  for (const id of ["A", "B"]) setOpenFileTabs(id, [{ path: "F.txt", name: "F.txt" }]);
  read.mockResolvedValue({ content: "stored F" });
  const hook = setup();
  await waitFor(() => expect(hook.result.current.tabs.map((file) => file.path)).toEqual(["F.txt"]));
  hook.rerender({ session: "A" });
  await waitFor(() => expect(hook.result.current.tabs.map((file) => file.path)).toEqual(["F.txt"]));
});

it.each(["changes", "pr"])(
  "preserves the valid initial non-Chat tab %s without file metadata",
  async (tab) => {
    setActiveTabForSession("B", tab);
    const hook = setup();
    await waitFor(() => expect(hook.result.current.tab).toBe(tab));
  },
);
