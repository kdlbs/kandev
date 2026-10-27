import { act, cleanup, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import type { ReactNode } from "react";
import type { DockviewApi } from "dockview-react";
import { StateProvider, useAppStore, useAppStoreApi } from "@/components/state-provider";
import { useDockviewStore } from "@/lib/state/dockview-store";
import { useFileEditors } from "./use-file-editors";
import { useFileOperations } from "./use-file-operations";

const mocks = vi.hoisted(() => ({ rename: vi.fn(), save: vi.fn(), read: vi.fn(), toast: vi.fn() }));
vi.mock("@/lib/ws/connection", () => ({ getWebSocketClient: () => ({}) }));
vi.mock("@/lib/ws/workspace-files", () => ({
  createFile: vi.fn(),
  deleteFile: vi.fn(),
  renameFile: (...args: unknown[]) => mocks.rename(...args),
  requestFileContent: (...args: unknown[]) => mocks.read(...args),
  updateFileContent: (...args: unknown[]) => mocks.save(...args),
}));
vi.mock("@/components/toast-provider", () => ({ useToast: () => ({ toast: mocks.toast }) }));
vi.mock("@/hooks/domains/session/use-session-git-status", () => ({
  useSessionGitStatus: () => undefined,
}));
vi.mock("@/lib/lsp/lsp-client-manager", () => ({ lspClientManager: { saveDocument: vi.fn() } }));
vi.mock("@/components/editors/monaco/monaco-init", () => ({ getMonacoInstance: () => null }));

function wrapper({ children }: { children: ReactNode }) {
  return <StateProvider>{children}</StateProvider>;
}
function host() {
  type Group = { id: string; panels: Panel[]; activePanel?: Panel };
  type Panel = {
    id: string;
    params: Record<string, unknown>;
    title: string;
    group: Group;
    view: { contentComponent: string; tabComponent: string };
    setTitle: (title: string) => void;
    toJSON: () => Record<string, never>;
    api: { setActive: () => void; updateParameters: (params: Record<string, unknown>) => void };
  };
  const panels = new Map<string, Panel>();
  const removed = new Set<(panel: Panel) => void>();
  const group: Group = { id: "center", panels: [] };
  let active: Panel | undefined;
  function panel(id: string, params: Record<string, unknown>, title: string): Panel {
    const item: Panel = {
      id,
      params,
      title,
      group,
      view: { contentComponent: "file-editor", tabComponent: "pinnedDefaultTab" },
      setTitle: (next) => {
        item.title = next;
      },
      toJSON: () => ({}),
      api: {
        setActive: () => {
          active = item;
          group.activePanel = item;
        },
        updateParameters: (next) => {
          item.params = next;
        },
      },
    };
    return item;
  }
  const preview = panel(
    "preview:file-editor",
    { path: "old.txt", previewItemId: "old.txt" },
    "old.txt",
  );
  panels.set(preview.id, preview);
  group.panels.push(preview);
  preview.api.setActive();
  const api = {
    get panels() {
      return [...panels.values()];
    },
    get activePanel() {
      return active;
    },
    groups: [group],
    getGroup: () => group,
    getPanel: (id: string) => panels.get(id),
    onDidActivePanelChange: () => ({ dispose: vi.fn() }),
    onDidRemovePanel: (listener: (value: Panel) => void) => {
      removed.add(listener);
      return { dispose: () => removed.delete(listener) };
    },
    addPanel: (options: {
      id: string;
      params: Record<string, unknown>;
      title: string;
      position?: { index?: number };
      inactive?: boolean;
    }) => {
      const added = panel(options.id, options.params, options.title);
      panels.set(added.id, added);
      group.panels.splice(options.position?.index ?? group.panels.length, 0, added);
      if (!options.inactive) added.api.setActive();
      return added;
    },
    removePanel: (item: Panel) => {
      panels.delete(item.id);
      group.panels = group.panels.filter((current) => current !== item);
      for (const listener of removed) listener(item);
    },
  };
  return { api: api as unknown as DockviewApi, panels, preview };
}

beforeEach(() => {
  vi.clearAllMocks();
  sessionStorage.clear();
  useDockviewStore.getState().clearFileStates();
  useDockviewStore.setState({ api: null, centerGroupId: "center", isRestoringLayout: false });
});
afterEach(() => {
  cleanup();
  useDockviewStore.setState({ api: null });
});

// @covers AC-UI-FILE-EDITOR-MUTATION-002.1, AC-UI-FILE-EDITOR-MUTATION-001.2
it.each([false, true])(
  "retargets the same buffer after preview materialization (close other preview: %s)",
  async (closeOtherPreview) => {
    const hst = host();
    useDockviewStore.setState({ api: hst.api });
    const h = renderHook(
      () => {
        const session = useAppStore((state) => state.tasks.activeSessionId);
        return {
          editors: useFileEditors(),
          rename: useFileOperations(session).renameFile,
          store: useAppStoreApi(),
        };
      },
      { wrapper },
    );
    act(() =>
      h.result.current.store.setState((state) => ({
        tasks: { ...state.tasks, activeSessionId: "promotion-session" },
        environmentIdBySessionId: { ...state.environmentIdBySessionId, "promotion-session": "env" },
      })),
    );
    act(() =>
      useDockviewStore.getState().setFileState("old.txt", {
        path: "old.txt",
        name: "old.txt",
        content: "disk",
        originalContent: "disk",
        originalHash: "hash",
        isDirty: false,
      }),
    );
    const instance = useDockviewStore.getState().openFiles.get("old.txt")?.instanceId;
    let resolve!: (response: { success: boolean }) => void;
    mocks.rename.mockReturnValueOnce(
      new Promise((done) => {
        resolve = done;
      }),
    );
    let pending!: Promise<boolean>;
    act(() => {
      pending = h.result.current.rename("old.txt", "new.txt");
    });
    act(() => h.result.current.editors.handleFileChange("old.txt", "typed during rename"));
    expect(hst.preview.params.promoted).toBe(true);
    mocks.read.mockResolvedValueOnce({ path: "other.txt", content: "other", is_binary: false });
    await act(async () => {
      await h.result.current.editors.openFile("other.txt");
    });
    expect(hst.panels.has("file:old.txt")).toBe(true);
    expect(useDockviewStore.getState().openFiles.get("old.txt")?.instanceId).toBe(instance);
    if (closeOtherPreview) {
      act(() =>
        hst.api.removePanel(hst.preview as unknown as Parameters<DockviewApi["removePanel"]>[0]),
      );
      expect(hst.panels.has("preview:file-editor")).toBe(false);
    }
    await act(async () => {
      resolve({ success: true });
      await pending;
    });
    expect(useDockviewStore.getState().openFiles.get("new.txt")).toMatchObject({
      content: "typed during rename",
      originalContent: "disk",
      isDirty: true,
      instanceId: instance,
    });
    expect(hst.panels.has("file:new.txt")).toBe(true);
    expect(hst.panels.has("file:old.txt")).toBe(false);
    mocks.save.mockResolvedValueOnce({ success: true, new_hash: "new-hash" });
    await act(async () => {
      await h.result.current.editors.saveFile("new.txt");
    });
    expect(mocks.save).toHaveBeenCalledWith(
      expect.anything(),
      "promotion-session",
      expect.objectContaining({ path: "new.txt", desiredContent: "typed during rename" }),
    );
  },
);

// @covers AC-UI-FILE-EDITOR-MUTATION-002.3
it("does not retarget a closed and reopened file after an older rename completes", async () => {
  const hst = host();
  useDockviewStore.setState({ api: hst.api });
  const h = renderHook(
    () => {
      const session = useAppStore((state) => state.tasks.activeSessionId);
      return {
        editors: useFileEditors(),
        rename: useFileOperations(session).renameFile,
        store: useAppStoreApi(),
      };
    },
    { wrapper },
  );
  act(() =>
    h.result.current.store.setState((state) => ({
      tasks: { ...state.tasks, activeSessionId: "reopen-session" },
    })),
  );
  const file = {
    path: "old.txt",
    name: "old.txt",
    content: "disk",
    originalContent: "disk",
    originalHash: "hash",
    isDirty: false,
  };
  act(() => useDockviewStore.getState().setFileState("old.txt", file));
  const previous = useDockviewStore.getState().openFiles.get("old.txt")?.instanceId;
  let resolve!: (response: { success: boolean }) => void;
  mocks.rename.mockReturnValueOnce(
    new Promise((done) => {
      resolve = done;
    }),
  );
  let pending!: Promise<boolean>;
  act(() => {
    pending = h.result.current.rename("old.txt", "new.txt");
  });
  act(() =>
    hst.api.removePanel(hst.preview as unknown as Parameters<DockviewApi["removePanel"]>[0]),
  );
  expect(useDockviewStore.getState().openFiles.has("old.txt")).toBe(false);
  act(() => {
    hst.api.addPanel({
      id: "file:old.txt",
      component: "file-editor",
      title: "old.txt",
      params: { path: "old.txt" },
      position: { referenceGroup: "center" },
    });
    useDockviewStore
      .getState()
      .setFileState("old.txt", { ...file, content: "replacement", isDirty: true });
  });
  expect(useDockviewStore.getState().openFiles.get("old.txt")?.instanceId).not.toBe(previous);
  await act(async () => {
    resolve({ success: true });
    await pending;
  });
  expect(useDockviewStore.getState().openFiles.get("old.txt")?.content).toBe("replacement");
  expect(useDockviewStore.getState().openFiles.has("new.txt")).toBe(false);
});

// @covers AC-UI-FILE-EDITOR-MUTATION-002.3
it("does not retarget a buffer after its Dockview host is replaced", async () => {
  const original = host();
  useDockviewStore.setState({ api: original.api });
  const h = renderHook(
    () => {
      const session = useAppStore((state) => state.tasks.activeSessionId);
      return {
        editors: useFileEditors(),
        rename: useFileOperations(session).renameFile,
        store: useAppStoreApi(),
      };
    },
    { wrapper },
  );
  act(() =>
    h.result.current.store.setState((state) => ({
      tasks: { ...state.tasks, activeSessionId: "host-session" },
    })),
  );
  act(() =>
    useDockviewStore.getState().setFileState("old.txt", {
      path: "old.txt",
      name: "old.txt",
      content: "draft",
      originalContent: "disk",
      originalHash: "hash",
      isDirty: true,
    }),
  );
  let resolve!: (response: { success: boolean }) => void;
  mocks.rename.mockReturnValueOnce(
    new Promise((done) => {
      resolve = done;
    }),
  );
  let pending!: Promise<boolean>;
  act(() => {
    pending = h.result.current.rename("old.txt", "new.txt");
  });
  const replacement = host();
  act(() => useDockviewStore.setState({ api: replacement.api }));
  await act(async () => {
    resolve({ success: true });
    await pending;
  });
  expect(useDockviewStore.getState().openFiles.get("old.txt")?.content).toBe("draft");
  expect(useDockviewStore.getState().openFiles.has("new.txt")).toBe(false);
  expect(replacement.panels.has("preview:file-editor")).toBe(true);
});
