import { StrictMode, useCallback, useState } from "react";
import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import type { OpenFileTab } from "@/lib/types/backend";
import { useFileSaveDelete, useFileTabRestoration } from "./task-center-panel-restoration";
import { useLiveFileTabs } from "./task-center-panel-state";
import {
  getFileTabKey,
  installFileEditorTab,
  upsertOpenFileTab,
} from "./task-center-panel-file-tabs";
import { createAppStore } from "@/lib/state/store";
import {
  setOpenFileTabs,
  setActiveTabForSession,
  getActiveTabForSession,
} from "@/lib/local-storage";

import { useStore } from "zustand";
const mocks = vi.hoisted(() => ({
  save: vi.fn(),
  delete: vi.fn(),
  read: vi.fn(),
  toast: vi.fn(),
  lsp: vi.fn(),
}));
vi.mock("@/lib/ws/connection", () => ({ getWebSocketClient: () => ({}) }));
vi.mock("@/lib/ws/workspace-files", () => ({
  updateFileContent: (...args: unknown[]) => mocks.save(...args),
  deleteFile: (...args: unknown[]) => mocks.delete(...args),
  requestFileContent: (...args: unknown[]) => mocks.read(...args),
}));
vi.mock("@/components/toast-provider", () => ({ useToast: () => ({ toast: mocks.toast }) }));
vi.mock("@/lib/lsp/lsp-client-manager", () => ({
  lspClientManager: { saveDocument: (...args: unknown[]) => mocks.lsp(...args) },
}));

const PATH = "src/foo.ts";
const REPO = "frontend";
type Reply = { success: boolean; new_hash?: string; error?: string };
function defer() {
  let resolve!: (reply: Reply) => void;
  let reject!: (error: Error) => void;
  const promise = new Promise<Reply>((yes, no) => {
    resolve = yes;
    reject = no;
  });
  return { promise, resolve, reject };
}
function file(label: string, repo = REPO): OpenFileTab {
  return {
    path: PATH,
    repo,
    name: "foo.ts",
    content: `${label} buffer`,
    originalContent: `${label} disk`,
    originalHash: `hash-${label}`,
    isDirty: true,
  };
}
function setup() {
  const hook = renderHook(
    ({ session }) => {
      const [tabs, setTabs, currentTabs] = useLiveFileTabs();
      const [leftTab, setLeftTab] = useState("chat");
      useFileTabRestoration({
        activeSessionId: session,
        leftTab,
        setLeftTab,
        setOpenFileTabs: setTabs,
        currentTabs,
      });
      const close = useCallback((key: string, instanceId?: symbol) => {
        setTabs((prev) =>
          prev.filter(
            (tab) =>
              getFileTabKey(tab) !== key ||
              (instanceId !== undefined && tab.instanceId !== instanceId),
          ),
        );
        setLeftTab("chat");
      }, []);
      const actions = useFileSaveDelete({
        activeSessionId: session,
        openFileTabs: tabs,
        setOpenFileTabs: setTabs,
        handleCloseFileTab: close,
      });
      return { tabs, setTabs, leftTab, setLeftTab, ...actions };
    },
    { initialProps: { session: "A" as string | null }, wrapper: StrictMode },
  );
  const seed = (label = "A", repo = REPO) =>
    act(() => {
      hook.result.current.setTabs((prev) => upsertOpenFileTab(prev, file(label, repo)));
      hook.result.current.setLeftTab(`file:${getFileTabKey(file(label, repo))}`);
    });
  return { ...hook, seed };
}
function start(action: () => Promise<void>) {
  let pending!: Promise<void>;
  act(() => {
    pending = action();
  });
  return pending;
}
async function settle(
  reply: ReturnType<typeof defer>,
  pending: Promise<void>,
  value: Reply = {
    success: true,
    new_hash: "saved-A",
  },
) {
  await act(async () => {
    reply.resolve(value);
    await pending;
  });
}
beforeEach(() => {
  vi.clearAllMocks();
  mocks.read.mockReset();
  sessionStorage.clear();
});
afterEach(cleanup);

// @covers AC-UI-FILE-EDITOR-MUTATION-001.1
it("tablet rejects old save after session navigation", async () => {
  const h = setup();
  h.seed();
  const reply = defer();
  mocks.save.mockReturnValueOnce(reply.promise);
  const pending = start(() => h.result.current.handleFileSave(PATH, REPO));
  h.rerender({ session: "B" });
  h.seed("B");
  await settle(reply, pending);
  expect(h.result.current.tabs[0]).toMatchObject(file("B"));
  expect(mocks.lsp).not.toHaveBeenCalled();
  expect(mocks.toast).not.toHaveBeenCalled();
});
// @covers AC-UI-FILE-EDITOR-MUTATION-001.1, AC-UI-FILE-EDITOR-MUTATION-001.4
it("tablet keeps replacement tab after old delete", async () => {
  const h = setup();
  h.seed();
  const reply = defer();
  mocks.delete.mockReturnValueOnce(reply.promise);
  const pending = start(() => h.result.current.handleFileDelete(PATH, REPO));
  h.rerender({ session: "B" });
  h.seed("B");
  await settle(reply, pending);
  expect(h.result.current.tabs[0]).toMatchObject(file("B"));
  expect(h.result.current.leftTab).toBe(`file:${getFileTabKey(file("B"))}`);
});

// @covers AC-UI-FILE-EDITOR-MUTATION-001.1, AC-UI-FILE-EDITOR-MUTATION-001.2
it.each(["return", "reopen", "replace", "unmount", "no session"])(
  "tablet retires save on %s",
  async (lifecycle) => {
    const h = setup();
    h.seed();
    const reply = defer();
    mocks.save.mockReturnValueOnce(reply.promise);
    const pending = start(() => h.result.current.handleFileSave(PATH, REPO));
    if (lifecycle === "return") {
      h.rerender({ session: "B" });
      h.rerender({ session: "A" });
      h.seed();
    } else if (lifecycle === "reopen") {
      act(() => h.result.current.setTabs([]));
      h.seed();
    } else if (lifecycle === "replace") {
      act(() => h.result.current.setTabs([installFileEditorTab(file("A"))]));
    } else if (lifecycle === "unmount") h.unmount();
    else h.rerender({ session: null });
    await settle(reply, pending);
    expect(h.result.current.tabs[0]?.originalHash).toBe("hash-A");
    expect(mocks.lsp).not.toHaveBeenCalled();
    expect(mocks.toast).not.toHaveBeenCalled();
    if (lifecycle !== "unmount") expect(h.result.current.savingFiles.size).toBe(0);
  },
);

// @covers AC-UI-FILE-EDITOR-MUTATION-001.2, AC-UI-FILE-EDITOR-MUTATION-001.4
it("tablet old delete cannot close an identical reopened tab or a later absent-owner tab", async () => {
  const h = setup();
  h.seed();
  const reply = defer();
  mocks.delete.mockReturnValueOnce(reply.promise);
  const pending = start(() => h.result.current.handleFileDelete(PATH, REPO));
  act(() => h.result.current.setTabs([]));
  h.seed();
  await settle(reply, pending);
  expect(h.result.current.tabs[0]).toMatchObject(file("A"));
  act(() => h.result.current.setTabs([]));
  await act(async () => h.result.current.handleFileDelete(PATH, REPO));
  h.seed("B");
  expect(mocks.delete).toHaveBeenCalledTimes(1);
  expect(h.result.current.tabs[0]).toMatchObject(file("B"));
});

// @covers AC-UI-FILE-EDITOR-MUTATION-001.3, AC-UI-FILE-EDITOR-MUTATION-001.5
it.each([false, true])(
  "tablet publishes saved snapshot and preserves typing=%s and other repos",
  async (typing) => {
    const h = setup();
    h.seed();
    h.seed("other", "backend");
    const reply = defer();
    mocks.save.mockReturnValueOnce(reply.promise);
    const pending = start(() => h.result.current.handleFileSave(PATH, REPO));
    expect(h.result.current.savingFiles).toEqual(new Set([getFileTabKey(file("A"))]));
    if (typing)
      act(() =>
        h.result.current.setTabs((prev) =>
          prev.map((tab) =>
            tab.repo === REPO ? { ...tab, content: "newer typing", isDirty: true } : tab,
          ),
        ),
      );
    await settle(reply, pending);
    expect(h.result.current.tabs[0]).toMatchObject({
      originalContent: "A buffer",
      originalHash: "saved-A",
      content: typing ? "newer typing" : "A buffer",
      isDirty: typing,
    });
    expect(h.result.current.tabs[1]).toMatchObject(file("other", "backend"));
    expect(mocks.lsp).toHaveBeenCalledWith(
      "A",
      PATH,
      REPO,
      "A buffer",
      typing ? "newer typing" : "A buffer",
    );
    expect(mocks.save.mock.calls[0][2]).toMatchObject({
      path: PATH,
      repo: REPO,
      desiredContent: "A buffer",
      originalHash: "hash-A",
    });
    expect(mocks.save.mock.calls[0][2]).not.toHaveProperty("instanceId");
    expect(h.result.current.savingFiles.size).toBe(0);
  },
);

// @covers AC-UI-FILE-EDITOR-MUTATION-001.5
it("tablet retired save cannot clear a replacement save's spinner", async () => {
  const h = setup();
  h.seed();
  const old = defer();
  const fresh = defer();
  mocks.save.mockReturnValueOnce(old.promise).mockReturnValueOnce(fresh.promise);
  const pending = start(() => h.result.current.handleFileSave(PATH, REPO));
  act(() => h.result.current.setTabs([]));
  h.seed("B");
  expect(h.result.current.savingFiles.size).toBe(0);
  const next = start(() => h.result.current.handleFileSave(PATH, REPO));
  await settle(old, pending);
  expect(h.result.current.savingFiles.has(getFileTabKey(file("B")))).toBe(true);
  expect(h.result.current.tabs[0]).toMatchObject(file("B"));
  await settle(fresh, next, { success: true, new_hash: "saved-B" });
  expect(h.result.current.savingFiles.size).toBe(0);
  expect(h.result.current.tabs[0]?.originalHash).toBe("saved-B");
});

// @covers AC-UI-FILE-EDITOR-MUTATION-001.4, AC-UI-FILE-EDITOR-MUTATION-001.5
it("tablet owned delete closes only its repository tab", async () => {
  const h = setup();
  h.seed();
  h.seed("other", "backend");
  mocks.delete.mockResolvedValueOnce({ success: true });
  await act(async () => h.result.current.handleFileDelete(PATH, REPO));
  expect(h.result.current.tabs).toMatchObject([file("other", "backend")]);
  expect(mocks.delete).toHaveBeenCalledWith(expect.anything(), "A", PATH, REPO);
});

// @covers AC-UI-FILE-EDITOR-MUTATION-001.1, AC-UI-FILE-EDITOR-MUTATION-001.4
it.each(["save", "delete"] as const)(
  "tablet suppresses retired %s failure but reports current rejection and exception",
  async (action) => {
    const h = setup();
    h.seed();
    const transport = action === "save" ? mocks.save : mocks.delete;
    const invoke = () =>
      action === "save"
        ? h.result.current.handleFileSave(PATH, REPO)
        : h.result.current.handleFileDelete(PATH, REPO);
    for (const throws of [false, true]) {
      const reply = defer();
      transport.mockReturnValueOnce(reply.promise);
      const pending = start(invoke);
      h.rerender({ session: "B" });
      h.seed("B");
      await act(async () => {
        if (throws) reply.reject(new Error("transport failed"));
        else reply.resolve({ success: false, error: "write failed" });
        await pending;
      });
      expect(mocks.toast).not.toHaveBeenCalled();
      expect(h.result.current.tabs[0]).toMatchObject(file("B"));
      h.rerender({ session: "A" });
      h.seed();
    }
    transport.mockResolvedValueOnce({ success: false, error: "write failed" });
    await act(async () => invoke());
    expect(mocks.toast).toHaveBeenCalledWith(
      expect.objectContaining({ description: "write failed" }),
    );
    transport.mockRejectedValueOnce(new Error("transport failed"));
    await act(async () => invoke());
    expect(mocks.toast).toHaveBeenCalledWith(
      expect.objectContaining({ description: "transport failed" }),
    );
    expect(h.result.current.tabs[0]).toMatchObject(file("A"));
    expect(mocks.lsp).not.toHaveBeenCalled();
    expect(h.result.current.savingFiles.size).toBe(0);
  },
);

it.each(["chat", "file", "legacy"] as const)(
  "restores file metadata without replacing the current tablet tab: %s",
  async (winner) => {
    const store = createAppStore();
    const fileKey = `file:${getFileTabKey(file("A"))}`;
    store.getState().setTabletSessionTab("A", fileKey);
    setOpenFileTabs("A", [{ path: PATH, name: "foo.ts", repo: REPO }]);
    setActiveTabForSession("A", fileKey);
    const read = Promise.withResolvers<{ content: string }>();
    mocks.read.mockReturnValue(read.promise);
    const hook = renderHook(() => {
      const [tabs, setTabs, currentTabs] = useLiveFileTabs();
      const [selected, setSelected] = useState(fileKey);
      useFileTabRestoration({
        store: winner === "legacy" ? undefined : store,
        activeSessionId: "A",
        leftTab: selected,
        setLeftTab: setSelected,
        setOpenFileTabs: setTabs,
        currentTabs,
      });
      return { tabs, selected, setSelected };
    });
    if (winner === "chat")
      act(() => {
        store.getState().setTabletSessionTab("A", "chat");
        hook.result.current.setSelected("chat");
      });
    await act(async () => read.resolve({ content: "restored disk content" }));
    await waitFor(() => expect(hook.result.current.tabs[0]?.content).toBe("restored disk content"));
    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 0));
    });
    expect(hook.result.current.selected).toBe(winner === "chat" ? "chat" : fileKey);
    hook.unmount();
  },
);

it("keeps a newly opened dirty tablet file when older saved metadata arrives", async () => {
  const store = createAppStore();
  const oldKey = `file:${getFileTabKey(file("A"))}`;
  store.getState().setTabletSessionTab("A", oldKey);
  setOpenFileTabs("A", [{ path: PATH, name: "foo.ts", repo: REPO }]);
  setActiveTabForSession("A", oldKey);
  const read = Promise.withResolvers<{ content: string }>();
  mocks.read.mockReturnValue(read.promise);
  const hook = renderHook(() => {
    const [tabs, setTabs, currentTabs] = useLiveFileTabs();
    const [selected, setSelected] = useState(oldKey);
    useFileTabRestoration({
      store,
      activeSessionId: "A",
      leftTab: selected,
      setLeftTab: setSelected,
      setOpenFileTabs: setTabs,
      currentTabs,
    });
    return { tabs, setTabs, selected, setSelected };
  });
  const live = installFileEditorTab({
    ...file("draft"),
    path: "src/live.ts",
    name: "live.ts",
    isDirty: true,
  });
  const liveKey = `file:${getFileTabKey(live)}`;
  act(() => {
    hook.result.current.setTabs([live]);
    store.getState().setTabletSessionTab("A", liveKey);
    hook.result.current.setSelected(liveKey);
  });
  await act(async () => read.resolve({ content: "old saved disk" }));
  await waitFor(() => expect(hook.result.current.tabs.some((tab) => tab.path === PATH)).toBe(true));
  await act(async () => {
    await new Promise((resolve) => setTimeout(resolve, 0));
  });
  expect(hook.result.current.tabs.find((tab) => tab.path === live.path)).toBe(live);
  expect(hook.result.current.tabs.find((tab) => tab.path === live.path)?.content).toBe(
    "draft buffer",
  );
  expect(hook.result.current.selected).toBe(liveKey);
  hook.unmount();
});

it("never writes the new tablet session's choice to its predecessor", () => {
  const store = createAppStore();
  const oldKey = `file:${getFileTabKey(file("A"))}`;
  store.getState().setTabletSessionTab("A", oldKey);
  store.getState().setTabletSessionTab("B", "chat");
  setActiveTabForSession("A", oldKey);
  setActiveTabForSession("B", "chat");
  const hook = renderHook(
    ({ session, selected }) => {
      const [tabs, setTabs, currentTabs] = useLiveFileTabs();
      useFileTabRestoration({
        store,
        activeSessionId: session,
        leftTab: selected,
        setLeftTab: () => {},
        setOpenFileTabs: setTabs,
        currentTabs,
      });
      return tabs;
    },
    { initialProps: { session: "A", selected: oldKey } },
  );
  hook.rerender({ session: "B", selected: "chat" });
  hook.unmount();
  expect(getActiveTabForSession("A", "chat")).toBe(oldKey);
  expect(getActiveTabForSession("B", "chat")).toBe("chat");
});

it("rejects saved metadata from an older A visit after A-B-A navigation", async () => {
  const store = createAppStore();
  const key = `file:${getFileTabKey(file("A"))}`;
  store.getState().setTabletSessionTab("A", key);
  store.getState().setTabletSessionTab("B", "chat");
  setOpenFileTabs("A", [{ path: PATH, name: "foo.ts", repo: REPO }]);
  setActiveTabForSession("A", key);
  const old = Promise.withResolvers<{ content: string }>();
  const fresh = Promise.withResolvers<{ content: string }>();
  mocks.read.mockReturnValueOnce(old.promise).mockReturnValueOnce(fresh.promise);
  const hook = renderHook(
    ({ session }) => {
      const [tabs, setTabs, currentTabs] = useLiveFileTabs();
      const selected = useStore(
        store,
        (state) => state.tabletSession.activeTabBySessionId[session],
      );
      useFileTabRestoration({
        store,
        activeSessionId: session,
        leftTab: selected,
        setLeftTab: (tab) => store.getState().setTabletSessionTab(session, tab),
        setOpenFileTabs: setTabs,
        currentTabs,
      });
      return tabs;
    },
    { initialProps: { session: "A" } },
  );
  hook.rerender({ session: "B" });
  hook.rerender({ session: "A" });
  await act(async () => old.resolve({ content: "stale visit disk" }));
  expect(hook.result.current).toEqual([]);
  await act(async () => fresh.resolve({ content: "current visit disk" }));
  await waitFor(() => expect(hook.result.current[0]?.content).toBe("current visit disk"));
  hook.unmount();
});
