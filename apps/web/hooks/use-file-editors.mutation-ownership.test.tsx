import {
  mocks,
  PATH,
  REPO,
  type Reply,
  defer,
  panelHost,
  setup,
  current,
  start,
  settle,
} from "./file-editor-mutation.test-helpers";
import { act, cleanup, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { StateProvider } from "@/components/state-provider";
import { useDockviewStore } from "@/lib/state/dockview-store";
import { buildRepoScopedItemId } from "@/lib/state/dockview-panel-actions";
import * as storage from "@/lib/local-storage";
import * as fileDiff from "@/lib/utils/file-diff";
import { useFileEditors } from "./use-file-editors";

beforeEach(() => {
  vi.clearAllMocks();
  sessionStorage.clear();
  useDockviewStore.getState().clearFileStates();
  useDockviewStore.setState({ api: null, isRestoringLayout: false });
});
afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  useDockviewStore.setState({ api: null });
});

// @covers AC-UI-FILE-EDITOR-MUTATION-001.1
it("rejects old save after session navigation", async () => {
  const h = setup();
  h.seed();
  const reply = defer<Reply>();
  mocks.save.mockReturnValueOnce(reply.promise);
  const pending = start(() => h.result.current.actions.saveFile(PATH, REPO));
  h.navigate(h.sessionB);
  const panel = h.seed("B");
  const persist = vi.spyOn(storage, "setOpenFileTabs");
  await settle(reply, pending, { success: true, new_hash: "saved-A", resolution: "overwritten" });
  expect(current()).toMatchObject({
    content: "B buffer",
    originalContent: "B disk",
    originalHash: "hash-B",
    isDirty: true,
  });
  expect(panel.api.updateParameters).not.toHaveBeenCalled();
  expect(panel.setTitle).not.toHaveBeenCalled();
  expect(mocks.lsp).not.toHaveBeenCalled();
  expect(mocks.toast).not.toHaveBeenCalled();
  expect(persist).not.toHaveBeenCalled();
});

// @covers AC-UI-FILE-EDITOR-MUTATION-001.1, AC-UI-FILE-EDITOR-MUTATION-001.4
it.each([false, true])("keeps replacement panel after old delete (preview=%s)", async (preview) => {
  const h = setup();
  h.seed("A", REPO, preview);
  const reply = defer<Reply>();
  mocks.delete.mockReturnValueOnce(reply.promise);
  const pending = start(() => h.result.current.actions.deleteFile(PATH, REPO));
  h.navigate(h.sessionB);
  const panel = h.seed("B", REPO, preview);
  await settle(reply, pending);
  expect(h.host.panels.get(panel.id)).toBe(panel);
  expect(current()?.originalContent).toBe("B disk");
  expect(h.host.api.removePanel).not.toHaveBeenCalled();
});

it.each(["save", "delete"] as const)("retires %s on A-B-A return", async (action) => {
  const h = setup();
  h.seed();
  const reply = defer<Reply>();
  mocks[action].mockReturnValueOnce(reply.promise);
  const pending = start(() =>
    action === "save"
      ? h.result.current.actions.saveFile(PATH, REPO)
      : h.result.current.actions.deleteFile(PATH, REPO),
  );
  h.navigate(h.sessionB);
  h.navigate(h.sessionA);
  const panel = h.seed("restored A");
  await settle(reply, pending);
  expect(current()?.originalContent).toBe("restored A disk");
  expect(h.host.panels.get(panel.id)).toBe(panel);
  expect(mocks.lsp).not.toHaveBeenCalled();
});

it.each(["save", "delete"] as const)("retires %s on unmount", async (action) => {
  const h = setup();
  h.seed();
  const reply = defer<Reply>();
  mocks[action].mockReturnValueOnce(reply.promise);
  const pending = start(() =>
    action === "save"
      ? h.result.current.actions.saveFile(PATH, REPO)
      : h.result.current.actions.deleteFile(PATH, REPO),
  );
  h.unmount();
  const panel = h.seed("replacement");
  await settle(reply, pending);
  expect(current()?.originalContent).toBe("replacement disk");
  expect(h.host.panels.get(panel.id)).toBe(panel);
  expect(mocks.lsp).not.toHaveBeenCalled();
});

// @covers AC-UI-FILE-EDITOR-MUTATION-001.2
it.each(["save", "delete"] as const)(
  "retires %s when the identical file is reopened",
  async (action) => {
    const h = setup();
    const old = h.seed();
    const reply = defer<Reply>();
    mocks[action].mockReturnValueOnce(reply.promise);
    const pending = start(() =>
      action === "save"
        ? h.result.current.actions.saveFile(PATH, REPO)
        : h.result.current.actions.deleteFile(PATH, REPO),
    );
    act(() => h.host.api.removePanel(old));
    const panel = h.seed();
    expect(h.result.current.actions.savingFiles.size).toBe(0);
    await settle(reply, pending);
    expect(current()?.originalContent).toBe("A disk");
    expect(h.host.panels.get(panel.id)).toBe(panel);
    expect(mocks.lsp).not.toHaveBeenCalled();
  },
);

it("rejects a save after the panel host is replaced", async () => {
  const h = setup();
  h.seed();
  const reply = defer<Reply>();
  mocks.save.mockReturnValueOnce(reply.promise);
  const pending = start(() => h.result.current.actions.saveFile(PATH, REPO));
  const replacement = panelHost();
  act(() => useDockviewStore.setState({ api: replacement.dockApi }));
  expect(h.result.current.actions.savingFiles.size).toBe(0);
  await settle(reply, pending);
  expect(current()?.originalContent).toBe("A disk");
  expect(mocks.lsp).not.toHaveBeenCalled();
});

it("retires a save when no session is selected", async () => {
  const h = setup();
  h.seed();
  const reply = defer<Reply>();
  mocks.save.mockReturnValueOnce(reply.promise);
  const pending = start(() => h.result.current.actions.saveFile(PATH, REPO));
  h.navigate(null);
  await settle(reply, pending);
  expect(current()?.originalContent).toBe("A disk");
  expect(h.result.current.actions.savingFiles.size).toBe(0);
  expect(mocks.lsp).not.toHaveBeenCalled();
});

it("keeps preview promotion and another consumer within the same buffer lifetime", async () => {
  const h = setup();
  const panel = h.seed("A", REPO, true, { content: "A disk", isDirty: false });
  const installed = current();
  act(() => h.result.current.actions.handleFileChange(PATH, "first edit", REPO));
  expect(panel.params.promoted).toBe(true);
  const reply = defer<Reply>();
  mocks.save.mockReturnValueOnce(reply.promise);
  const pending = start(() => h.result.current.actions.saveFile(PATH, REPO));
  const consumer = renderHook(() => useFileEditors(), {
    wrapper: ({ children }) => (
      <StateProvider initialState={{ tasks: h.result.current.store.getState().tasks }}>
        {children}
      </StateProvider>
    ),
  });
  expect(current()?.instanceId).toBe(installed?.instanceId);
  consumer.unmount();
  await settle(reply, pending);
  expect(current()).toMatchObject({
    content: "first edit",
    originalContent: "first edit",
    isDirty: false,
  });
  expect(storage.getOpenFileTabs(h.sessionA)[0]?.pinned).toBe(true);
});

it("does not close a panel opened after a panel-less delete", async () => {
  const h = setup();
  const reply = defer<Reply>();
  mocks.delete.mockReturnValueOnce(reply.promise);
  const pending = start(() => h.result.current.actions.deleteFile(PATH, REPO));
  const panel = h.seed();
  await settle(reply, pending);
  expect(h.host.panels.get(panel.id)).toBe(panel);
});

// @covers AC-UI-FILE-EDITOR-MUTATION-001.3
it("preserves typing and the disk/LSP boundary while a save is pending", async () => {
  const h = setup();
  const panel = h.seed();
  const reply = defer<Reply>();
  mocks.save.mockReturnValueOnce(reply.promise);
  const pending = start(() => h.result.current.actions.saveFile(PATH, REPO));
  act(() => h.result.current.actions.handleFileChange(PATH, "v3 typed", REPO));
  await settle(reply, pending);
  expect(current()).toMatchObject({
    content: "v3 typed",
    originalContent: "A buffer",
    originalHash: "saved-A",
    isDirty: true,
  });
  expect(mocks.lsp).toHaveBeenCalledWith(h.sessionA, PATH, REPO, "A buffer", "v3 typed");
  expect(panel.api.updateParameters).not.toHaveBeenCalled();
  expect(h.result.current.actions.savingFiles.size).toBe(0);
});

it("clears an owned clean save and persists only tab descriptors", async () => {
  const h = setup();
  const panel = h.seed("A", REPO, true);
  mocks.save.mockResolvedValueOnce({
    success: true,
    new_hash: "saved-A",
    resolution: "overwritten",
  });
  await act(async () => h.result.current.actions.saveFile(PATH, REPO));
  expect(current()).toMatchObject({
    originalContent: "A buffer",
    originalHash: "saved-A",
    isDirty: false,
  });
  expect(panel.params.isDirty).toBe(false);
  expect(panel.setTitle).toHaveBeenCalledWith("foo.ts");
  expect(mocks.lsp).toHaveBeenCalledWith(h.sessionA, PATH, REPO, "A buffer", "A buffer");
  expect(mocks.toast).toHaveBeenCalledOnce();
  expect(storage.getOpenFileTabs(h.sessionA)).toEqual([
    { path: PATH, repo: REPO, name: "foo.ts", pinned: false },
  ]);
  expect(mocks.save.mock.calls[0][2]).toEqual(
    expect.objectContaining({
      path: PATH,
      repo: REPO,
      desiredContent: "A buffer",
      originalHash: "hash-A",
    }),
  );
  expect(Object.keys(mocks.save.mock.calls[0][2]).sort()).toEqual([
    "desiredContent",
    "diff",
    "originalHash",
    "path",
    "repo",
  ]);
});

// @covers AC-UI-FILE-EDITOR-MUTATION-001.4
it.each([false, true])(
  "closes the owned successful delete through removal wiring (preview=%s)",
  async (preview) => {
    const h = setup();
    const panel = h.seed("A", REPO, preview);
    mocks.delete.mockResolvedValueOnce({ success: true });
    await act(async () => h.result.current.actions.deleteFile(PATH, REPO));
    expect(h.host.api.removePanel).toHaveBeenCalledWith(panel);
    expect(current()).toBeUndefined();
    expect(storage.getOpenFileTabs(h.sessionA)).toEqual([]);
  },
);

it.each(["save", "delete"] as const)(
  "keeps owned %s failure feedback but suppresses retired failures",
  async (action) => {
    const h = setup();
    const panel = h.seed();
    mocks[action].mockResolvedValueOnce({ success: false, error: "disk unavailable" });
    await act(async () =>
      action === "save"
        ? h.result.current.actions.saveFile(PATH, REPO)
        : h.result.current.actions.deleteFile(PATH, REPO),
    );
    expect(mocks.toast).toHaveBeenCalledWith(
      expect.objectContaining({ description: "disk unavailable", variant: "error" }),
    );
    expect(current()?.originalContent).toBe("A disk");
    expect(h.host.panels.get(panel.id)).toBe(panel);
    mocks.toast.mockClear();
    const reply = defer<Reply>();
    mocks[action].mockReturnValueOnce(reply.promise);
    const pending = start(() =>
      action === "save"
        ? h.result.current.actions.saveFile(PATH, REPO)
        : h.result.current.actions.deleteFile(PATH, REPO),
    );
    h.navigate(h.sessionB);
    h.seed("B");
    await settle(reply, pending, { success: false, error: "old failure" });
    expect(mocks.toast).not.toHaveBeenCalled();
    expect(mocks.lsp).not.toHaveBeenCalled();
  },
);

it.each(["save", "delete"] as const)(
  "keeps owned %s exceptions but suppresses retired exceptions",
  async (action) => {
    const h = setup();
    h.seed();
    mocks[action].mockRejectedValueOnce(new Error("disk disconnected"));
    await act(async () =>
      action === "save"
        ? h.result.current.actions.saveFile(PATH, REPO)
        : h.result.current.actions.deleteFile(PATH, REPO),
    );
    expect(mocks.toast).toHaveBeenCalledWith(
      expect.objectContaining({ description: "disk disconnected" }),
    );
    expect(current()?.originalContent).toBe("A disk");
    mocks.toast.mockClear();
    const reply = defer<Reply>();
    mocks[action].mockReturnValueOnce(reply.promise);
    const pending = start(() =>
      action === "save"
        ? h.result.current.actions.saveFile(PATH, REPO)
        : h.result.current.actions.deleteFile(PATH, REPO),
    );
    h.navigate(h.sessionB);
    h.seed("B");
    await act(async () => {
      reply.reject(new Error("old disconnected"));
      await pending;
    });
    expect(mocks.toast).not.toHaveBeenCalled();
    expect(mocks.lsp).not.toHaveBeenCalled();
  },
);

// @covers AC-UI-FILE-EDITOR-MUTATION-001.5
it("does not clear the replacement save spinner when the old save settles", async () => {
  const h = setup();
  h.seed();
  const first = defer<Reply>();
  const second = defer<Reply>();
  mocks.save.mockReturnValueOnce(first.promise).mockReturnValueOnce(second.promise);
  const old = start(() => h.result.current.actions.saveFile(PATH, REPO));
  h.navigate(h.sessionB);
  h.seed("B");
  expect(h.result.current.actions.savingFiles.size).toBe(0);
  const next = start(() => h.result.current.actions.saveFile(PATH, REPO));
  await settle(first, old);
  expect(h.result.current.actions.savingFiles.has(buildRepoScopedItemId(PATH, REPO))).toBe(true);
  await settle(second, next, { success: true, new_hash: "saved-B" });
  expect(current()?.originalContent).toBe("B buffer");
  expect(h.result.current.actions.savingFiles.size).toBe(0);
});

it("routes the same path independently in two repositories", async () => {
  const h = setup();
  h.seed();
  const backend = h.seed("backend", "backend");
  const reply = defer<Reply>();
  mocks.save.mockReturnValueOnce(reply.promise);
  const pending = start(() => h.result.current.actions.saveFile(PATH, REPO));
  expect(h.result.current.actions.savingFiles.has(buildRepoScopedItemId(PATH, "backend"))).toBe(
    false,
  );
  await settle(reply, pending);
  expect(current("backend")?.originalContent).toBe("backend disk");
  expect(backend.api.updateParameters).not.toHaveBeenCalled();
  mocks.delete.mockResolvedValueOnce({ success: true });
  await act(async () => h.result.current.actions.deleteFile(PATH, "backend"));
  expect(mocks.delete).toHaveBeenCalledWith(mocks.client, h.sessionA, PATH, "backend");
  expect(current()?.originalContent).toBe("A buffer");
  expect(current("backend")).toBeUndefined();
});

// @covers AC-UI-FILE-EDITOR-MUTATION-001.6
it("applies the current remote snapshot and keeps a replacement safe from deferred hash work", async () => {
  const h = setup();
  h.seed("A", REPO, false, {
    hasRemoteUpdate: true,
    remoteContent: "remote",
    remoteOriginalHash: "remote-hash",
  });
  await act(async () => h.result.current.actions.applyRemoteUpdate(PATH, REPO));
  expect(current()).toMatchObject({
    content: "remote",
    originalContent: "remote",
    originalHash: "remote-hash",
    isDirty: false,
    hasRemoteUpdate: false,
  });
  const hash = defer<string>();
  vi.spyOn(fileDiff, "calculateHash").mockReturnValueOnce(hash.promise);
  h.seed("A", REPO, false, { hasRemoteUpdate: true, remoteContent: "old remote" });
  const pending = start(() => h.result.current.actions.applyRemoteUpdate(PATH, REPO));
  h.navigate(h.sessionB);
  h.seed("B");
  await act(async () => {
    hash.resolve("old hash");
    await pending;
  });
  expect(current()).toMatchObject({
    content: "B buffer",
    originalContent: "B disk",
    originalHash: "hash-B",
  });
});
