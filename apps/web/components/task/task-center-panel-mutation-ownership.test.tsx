import { StrictMode, useCallback, useState } from "react";
import { act, cleanup, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import type { OpenFileTab } from "@/lib/types/backend";
import { useFileSaveDelete, useFileTabRestoration } from "./task-center-panel-restoration";
import {
  getFileTabKey,
  installFileEditorTab,
  upsertOpenFileTab,
  type FileEditorTab,
} from "./task-center-panel-file-tabs";

const mocks = vi.hoisted(() => ({ save: vi.fn(), delete: vi.fn(), toast: vi.fn(), lsp: vi.fn() }));
vi.mock("@/lib/ws/connection", () => ({ getWebSocketClient: () => ({}) }));
vi.mock("@/lib/ws/workspace-files", () => ({
  updateFileContent: (...args: unknown[]) => mocks.save(...args),
  deleteFile: (...args: unknown[]) => mocks.delete(...args),
  requestFileContent: vi.fn(),
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
      const [tabs, setTabs] = useState<FileEditorTab[]>([]);
      const [leftTab, setLeftTab] = useState("chat");
      useFileTabRestoration({
        activeSessionId: session,
        leftTab,
        setLeftTab,
        setOpenFileTabs: setTabs,
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
