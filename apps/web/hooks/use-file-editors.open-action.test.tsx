import { renderHook, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { FileContentResponse } from "@/lib/types/backend";
import type { FileEditorState } from "@/lib/state/dockview-store";

const mocks = vi.hoisted(() => {
  const fakeClient = {};
  const toast = vi.fn();
  const requestFileContent = vi.fn();
  const addFileEditorPanel = vi.fn();
  const promotePreviewToPinned = vi.fn();

  type MockFileState = {
    path: string;
    repo?: string;
    name: string;
    content: string;
    originalContent: string;
    originalHash: string;
    isDirty: boolean;
    isBinary?: boolean;
    markdownPreview?: boolean;
  };

  type DockState = {
    api: null;
    openFiles: Map<string, MockFileState>;
    isRestoringLayout: boolean;
    setFileState: (key: string, state: MockFileState) => void;
    updateFileState: (key: string, updates: Partial<MockFileState>) => void;
    removeFileState: (key: string) => void;
    clearFileStates: () => void;
    addFileEditorPanel: ReturnType<typeof vi.fn>;
    promotePreviewToPinned: ReturnType<typeof vi.fn>;
  };

  let dockState: DockState;

  const resetDockState = () => {
    dockState = {
      api: null,
      openFiles: new Map(),
      isRestoringLayout: false,
      setFileState: vi.fn((key: string, state: MockFileState) => {
        dockState.openFiles.set(key, state);
      }),
      updateFileState: vi.fn((key: string, updates: Partial<MockFileState>) => {
        const current = dockState.openFiles.get(key);
        if (current) dockState.openFiles.set(key, { ...current, ...updates });
      }),
      removeFileState: vi.fn((key: string) => {
        dockState.openFiles.delete(key);
      }),
      clearFileStates: vi.fn(() => {
        dockState.openFiles.clear();
      }),
      addFileEditorPanel,
      promotePreviewToPinned,
    };
  };

  resetDockState();

  return {
    fakeClient,
    toast,
    requestFileContent,
    addFileEditorPanel,
    promotePreviewToPinned,
    getDockState: () => dockState,
    resetDockState,
  };
});

vi.mock("@/components/editors/monaco/monaco-init", () => ({
  getMonacoInstance: () => null,
}));

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: { tasks: { activeSessionId: string } }) => unknown) =>
    selector({ tasks: { activeSessionId: "sess-1" } }),
}));

vi.mock("@/components/toast-provider", () => ({
  useToast: () => ({ toast: mocks.toast }),
}));

vi.mock("@/hooks/domains/session/use-session-git-status", () => ({
  useSessionGitStatus: () => null,
}));

vi.mock("@/lib/local-storage", () => ({
  getOpenFileTabs: () => [],
  setOpenFileTabs: vi.fn(),
  getActiveTabForSession: () => "chat",
  setActiveTabForSession: vi.fn(),
}));

vi.mock("@/lib/state/dockview-store", () => {
  const useDockviewStore = Object.assign(
    (selector: (state: ReturnType<typeof mocks.getDockState>) => unknown) =>
      selector(mocks.getDockState()),
    {
      getState: () => mocks.getDockState(),
      subscribe: vi.fn(() => vi.fn()),
    },
  );
  return { useDockviewStore };
});

vi.mock("@/lib/utils/file-diff", () => ({
  calculateHash: async (content: string) => `h:${content.length}`,
  generateUnifiedDiff: vi.fn(),
}));

vi.mock("@/lib/ws/connection", () => ({
  getWebSocketClient: () => mocks.fakeClient,
}));

vi.mock("@/lib/ws/workspace-files", () => ({
  requestFileContent: (...args: unknown[]) => mocks.requestFileContent(...args),
  updateFileContent: vi.fn(),
  deleteFile: vi.fn(),
}));

vi.mock("./file-editors-sync", () => ({
  useOpenFileWorkspaceSync: vi.fn(),
}));

import { useFileEditors } from "./use-file-editors";

const FIRST_PATH = "src/first.ts";
const SECOND_PATH = "src/second.ts";

function defer<T>() {
  let resolve: (value: T) => void = () => undefined;
  const promise = new Promise<T>((innerResolve) => {
    resolve = innerResolve;
  });
  return { promise, resolve };
}

function fileState(path: string, content: string): FileEditorState {
  return {
    path,
    name: path.split("/").pop() ?? path,
    content,
    originalContent: content,
    originalHash: `h:${content.length}`,
    isDirty: false,
  };
}

function fileResponse(path: string, content: string): FileContentResponse {
  return {
    path,
    content,
    is_binary: false,
  } as FileContentResponse;
}

describe("useFileEditors open actions", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mocks.resetDockState();
  });

  it("keeps a pending file open request alive when focusing an already-open file", async () => {
    const first = defer<FileContentResponse>();
    mocks.requestFileContent.mockReturnValueOnce(first.promise);
    const { result } = renderHook(() => useFileEditors());

    mocks.getDockState().openFiles.set(SECOND_PATH, fileState(SECOND_PATH, "second"));

    const pendingOpen = result.current.openFile(FIRST_PATH);
    await waitFor(() => expect(mocks.requestFileContent).toHaveBeenCalledTimes(1));

    await result.current.openFile(SECOND_PATH);

    first.resolve(fileResponse(FIRST_PATH, "first"));
    await pendingOpen;

    expect(mocks.getDockState().openFiles.get(FIRST_PATH)?.content).toBe("first");
    expect(mocks.addFileEditorPanel).toHaveBeenCalledWith(FIRST_PATH, "first.ts", {
      repo: undefined,
    });
  });

  it("keeps a pending markdown preview request alive when focusing an already-open file", async () => {
    const first = defer<FileContentResponse>();
    mocks.requestFileContent.mockReturnValueOnce(first.promise);
    const { result } = renderHook(() => useFileEditors());

    mocks.getDockState().openFiles.set(SECOND_PATH, fileState(SECOND_PATH, "second"));

    const pendingOpen = result.current.openFileInMarkdownPreview(FIRST_PATH);
    await waitFor(() => expect(mocks.requestFileContent).toHaveBeenCalledTimes(1));

    await result.current.openFileInMarkdownPreview(SECOND_PATH);

    first.resolve(fileResponse(FIRST_PATH, "first"));
    await pendingOpen;

    expect(mocks.getDockState().openFiles.get(FIRST_PATH)).toMatchObject({
      content: "first",
      markdownPreview: true,
    });
    expect(mocks.addFileEditorPanel).toHaveBeenCalledWith(FIRST_PATH, "first.ts", {
      repo: undefined,
    });
  });
});
