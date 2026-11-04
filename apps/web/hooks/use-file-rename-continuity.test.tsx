import { act, cleanup, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import type { ReactNode } from "react";
import type { DockviewApi } from "dockview-react";
import { StateProvider, useAppStore, useAppStoreApi } from "@/components/state-provider";
import { useDockviewStore } from "@/lib/state/dockview-store";
import { useFileEditors } from "./use-file-editors";
import { useFileOperations } from "./use-file-operations";

const mocks = vi.hoisted(() => ({ rename: vi.fn(), toast: vi.fn(), read: vi.fn() }));
vi.mock("@/lib/ws/connection", () => ({ getWebSocketClient: () => ({}) }));
vi.mock("@/lib/ws/workspace-files", () => ({
  createFile: vi.fn(),
  deleteFile: vi.fn(),
  renameFile: (...args: unknown[]) => mocks.rename(...args),
  requestFileContent: (...args: unknown[]) => mocks.read(...args),
  updateFileContent: vi.fn(),
}));
vi.mock("@/components/toast-provider", () => ({ useToast: () => ({ toast: mocks.toast }) }));
vi.mock("@/hooks/domains/session/use-session-git-status", () => ({
  useSessionGitStatus: () => undefined,
}));
vi.mock("@/lib/lsp/lsp-client-manager", () => ({
  lspClientManager: { saveDocument: vi.fn() },
}));
vi.mock("@/components/editors/monaco/monaco-init", () => ({ getMonacoInstance: () => null }));

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((done) => {
    resolve = done;
  });
  return { promise, resolve };
}

function wrapper({ children }: { children: ReactNode }) {
  return <StateProvider>{children}</StateProvider>;
}

beforeEach(() => {
  vi.clearAllMocks();
  sessionStorage.clear();
  useDockviewStore.getState().clearFileStates();
  useDockviewStore.setState({ api: null, isRestoringLayout: false });
});
afterEach(() => {
  cleanup();
  useDockviewStore.setState({ api: null });
});

// @covers AC-UI-FILE-EDITOR-MUTATION-002.1, AC-UI-FILE-EDITOR-MUTATION-002.3
it("publishes an accepted rename to a dirty preview only after the workspace reply", async () => {
  const panel: {
    id: string;
    params: Record<string, unknown>;
    setTitle: ReturnType<typeof vi.fn>;
    api: { updateParameters: ReturnType<typeof vi.fn> };
  } = {
    id: "preview:file-editor",
    params: { path: "old.txt", previewItemId: "old.txt", promoted: true, isDirty: true },
    setTitle: vi.fn(),
    api: {
      updateParameters: vi.fn((params: Record<string, unknown>) => {
        panel.params = params;
      }),
    },
  };
  const api = {
    getPanel: (id: string) => (id === panel.id ? panel : undefined),
    onDidRemovePanel: () => ({ dispose: vi.fn() }),
    onDidActivePanelChange: () => ({ dispose: vi.fn() }),
  };
  useDockviewStore.setState({ api: api as unknown as DockviewApi });
  const h = renderHook(
    () => {
      const session = useAppStore((state) => state.tasks.activeSessionId);
      useFileEditors();
      return { rename: useFileOperations(session).renameFile, store: useAppStoreApi() };
    },
    { wrapper },
  );
  act(() =>
    h.result.current.store.setState((state) => ({
      tasks: { ...state.tasks, activeSessionId: "rename-session" },
      environmentIdBySessionId: { ...state.environmentIdBySessionId, "rename-session": "env-a" },
    })),
  );
  act(() =>
    useDockviewStore.getState().setFileState("old.txt", {
      path: "old.txt",
      name: "old.txt",
      content: "unsaved text",
      originalContent: "disk text",
      originalHash: "disk-hash",
      isDirty: true,
    }),
  );
  const instance = useDockviewStore.getState().openFiles.get("old.txt")?.instanceId;
  mocks.rename.mockResolvedValueOnce({ success: false, error: "denied" });
  await act(async () => {
    expect(await h.result.current.rename("old.txt", "new.txt")).toBe(false);
  });
  expect(useDockviewStore.getState().openFiles.get("old.txt")?.instanceId).toBe(instance);
  expect(panel.params.path).toBe("old.txt");
  const reply = deferred<{ success: boolean }>();
  mocks.rename.mockReturnValueOnce(reply.promise);
  let pending!: Promise<boolean>;
  act(() => {
    pending = h.result.current.rename("old.txt", "new.txt");
  });
  expect(useDockviewStore.getState().openFiles.get("old.txt")?.content).toBe("unsaved text");
  await act(async () => {
    reply.resolve({ success: true });
    await pending;
  });
  expect(useDockviewStore.getState().openFiles.has("old.txt")).toBe(false);
  expect(useDockviewStore.getState().openFiles.get("new.txt")).toMatchObject({
    path: "new.txt",
    name: "new.txt",
    content: "unsaved text",
    originalContent: "disk text",
    originalHash: "disk-hash",
    isDirty: true,
    instanceId: instance,
  });
  expect(panel.params).toMatchObject({ path: "new.txt", previewItemId: "new.txt", promoted: true });
});

// @covers AC-UI-FILE-EDITOR-MUTATION-002.3, AC-UI-FILE-EDITOR-MUTATION-002.5
it("blocks destination opens while pending and does not retarget a replacement after A-B-A navigation", async () => {
  const preview = {
    id: "preview:file-editor",
    params: { path: "old.txt", previewItemId: "old.txt", promoted: true },
    setTitle: vi.fn(),
    api: { updateParameters: vi.fn() },
  };
  const api = {
    getPanel: (id: string) => (id === preview.id ? preview : undefined),
    onDidRemovePanel: () => ({ dispose: vi.fn() }),
    onDidActivePanelChange: () => ({ dispose: vi.fn() }),
  };
  useDockviewStore.setState({ api: api as unknown as DockviewApi });
  const h = renderHook(
    () => {
      const session = useAppStore((state) => state.tasks.activeSessionId);
      const editors = useFileEditors();
      return { rename: useFileOperations(session).renameFile, editors, store: useAppStoreApi() };
    },
    { wrapper },
  );
  const navigate = (session: string) =>
    act(() =>
      h.result.current.store.setState((state) => ({
        tasks: { ...state.tasks, activeSessionId: session },
        environmentIdBySessionId: { ...state.environmentIdBySessionId, A: "env-a", B: "env-b" },
      })),
    );
  const seed = (content: string) =>
    act(() =>
      useDockviewStore.getState().setFileState("old.txt", {
        path: "old.txt",
        name: "old.txt",
        content,
        originalContent: "disk",
        originalHash: "hash",
        isDirty: true,
      }),
    );
  navigate("A");
  seed("original dirty");
  const reply = deferred<{ success: boolean }>();
  mocks.rename.mockReturnValueOnce(reply.promise);
  let pending!: Promise<boolean>;
  act(() => {
    pending = h.result.current.rename("old.txt", "new.txt");
  });
  await act(async () => {
    await h.result.current.editors.openFile("new.txt");
  });
  expect(mocks.read).not.toHaveBeenCalled();
  navigate("B");
  navigate("A");
  seed("replacement dirty");
  await act(async () => {
    reply.resolve({ success: true });
    await pending;
  });
  expect(useDockviewStore.getState().openFiles.get("old.txt")?.content).toBe("replacement dirty");
  expect(useDockviewStore.getState().openFiles.has("new.txt")).toBe(false);
});

// @covers AC-UI-FILE-EDITOR-MUTATION-002.5
it("rejects a rename before transport when a distinct destination editor is already open", async () => {
  const preview = {
    id: "preview:file-editor",
    params: { path: "old.txt", previewItemId: "old.txt" },
    setTitle: vi.fn(),
    api: { updateParameters: vi.fn() },
  };
  const api = {
    getPanel: (id: string) => (id === preview.id ? preview : undefined),
    onDidRemovePanel: () => ({ dispose: vi.fn() }),
    onDidActivePanelChange: () => ({ dispose: vi.fn() }),
  };
  useDockviewStore.setState({ api: api as unknown as DockviewApi });
  const h = renderHook(
    () => {
      const session = useAppStore((state) => state.tasks.activeSessionId);
      useFileEditors();
      return { rename: useFileOperations(session).renameFile, store: useAppStoreApi() };
    },
    { wrapper },
  );
  act(() =>
    h.result.current.store.setState((state) => ({
      tasks: { ...state.tasks, activeSessionId: "A" },
    })),
  );
  act(() => {
    for (const path of ["old.txt", "new.txt"])
      useDockviewStore.getState().setFileState(path, {
        path,
        name: path,
        content: `${path} dirty`,
        originalContent: "disk",
        originalHash: "hash",
        isDirty: true,
      });
  });
  await act(async () => {
    expect(await h.result.current.rename("old.txt", "new.txt")).toBe(false);
  });
  expect(mocks.rename).not.toHaveBeenCalled();
  expect(useDockviewStore.getState().openFiles.get("old.txt")?.content).toBe("old.txt dirty");
  expect(useDockviewStore.getState().openFiles.get("new.txt")?.content).toBe("new.txt dirty");
});
