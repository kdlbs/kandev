import { act, cleanup, renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

const lspMocks = vi.hoisted(() => {
  const attachedRepository = "second-repository-main";
  return {
    attachedRepository,
    sessionId: "session-1",
    taskRoot: "/task-root",
    state: {
      tasks: { activeSessionId: "session-1" },
      taskSessions: {
        items: {
          "session-1": {
            workspace_path: "/task-root",
            worktree_path: "/task-root/kandev",
          },
        },
      },
    },
    openFile: vi.fn(),
    setPendingCursorPosition: vi.fn(),
    scrollEditorIfMounted: vi.fn(),
    disposePlaceholderModel: vi.fn(),
    setFileOpener: vi.fn(),
    getFileOpener: vi.fn(),
    getWorkspaceUriForSession: vi.fn(() => null),
    getRepositorySubpaths: vi.fn(() => [attachedRepository]),
    currentOpener: null as ((uri: string, line?: number, column?: number) => void) | null,
  };
});

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: typeof lspMocks.state) => unknown) => selector(lspMocks.state),
}));

vi.mock("@/hooks/use-file-editors", () => ({
  useFileEditors: () => ({ openFile: lspMocks.openFile }),
  setPendingCursorPosition: lspMocks.setPendingCursorPosition,
  scrollEditorIfMounted: lspMocks.scrollEditorIfMounted,
}));

vi.mock("@/lib/lsp/lsp-client-manager", () => ({
  lspClientManager: {
    disposePlaceholderModel: lspMocks.disposePlaceholderModel,
    setFileOpener: lspMocks.setFileOpener,
    getFileOpener: lspMocks.getFileOpener,
    getWorkspaceUriForSession: lspMocks.getWorkspaceUriForSession,
    getRepositorySubpaths: lspMocks.getRepositorySubpaths,
  },
}));

import { modelUriForDocument } from "@/lib/lsp/file-uri";
import { toWorkspaceRelativePath, useLspFileOpener } from "./use-lsp-file-opener";

const SOURCE_FILE_PATH = "src/index.ts";
const ATTACHED_FILE_PATH = `${lspMocks.attachedRepository}/${SOURCE_FILE_PATH}`;
const ATTACHED_FILE_URI = `file:///task-root/${ATTACHED_FILE_PATH}`;
const ATTACHED_MODEL_URI = modelUriForDocument(ATTACHED_FILE_URI, lspMocks.sessionId);
const NEXT_TASK_ROOT = "/next-task-root";

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
  lspMocks.currentOpener = null;
  lspMocks.state.taskSessions.items["session-1"].workspace_path = lspMocks.taskRoot;
});

describe("toWorkspaceRelativePath", () => {
  it("resolves attached-repository files from the task workspace root", () => {
    expect(toWorkspaceRelativePath(`/task-root/${ATTACHED_FILE_PATH}`, lspMocks.taskRoot)).toBe(
      ATTACHED_FILE_PATH,
    );
  });

  it("preserves sibling prefixes and handles a trailing workspace separator", () => {
    expect(toWorkspaceRelativePath("/task-root-old/src/index.ts", lspMocks.taskRoot)).toBe(
      "/task-root-old/src/index.ts",
    );
    expect(toWorkspaceRelativePath("/task-root/src/index.ts", `${lspMocks.taskRoot}/`)).toBe(
      SOURCE_FILE_PATH,
    );
  });

  it("keeps the absolute path when no workspace root is available", () => {
    expect(toWorkspaceRelativePath("/legacy-worktree/src/index.ts", null)).toBe(
      "/legacy-worktree/src/index.ts",
    );
  });
});

describe("useLspFileOpener", () => {
  it("registers an opener that uses the effective workspace path", async () => {
    lspMocks.setFileOpener.mockImplementation((opener) => {
      lspMocks.currentOpener = opener;
    });
    lspMocks.getFileOpener.mockImplementation(() => lspMocks.currentOpener);

    renderHook(() => useLspFileOpener());
    const opener = lspMocks.currentOpener;
    expect(opener).not.toBeNull();

    await act(async () => {
      await opener?.(ATTACHED_MODEL_URI, 12, 3);
    });

    expect(lspMocks.disposePlaceholderModel).toHaveBeenCalledWith(ATTACHED_MODEL_URI);
    expect(lspMocks.openFile).toHaveBeenCalledWith(SOURCE_FILE_PATH, lspMocks.attachedRepository);
    expect(lspMocks.setPendingCursorPosition).toHaveBeenCalledWith(
      SOURCE_FILE_PATH,
      12,
      3,
      lspMocks.attachedRepository,
      lspMocks.sessionId,
    );
    expect(lspMocks.scrollEditorIfMounted).toHaveBeenCalledWith(
      SOURCE_FILE_PATH,
      "file:///task-root",
      12,
      3,
      { repo: lspMocks.attachedRepository, sessionId: lspMocks.sessionId },
    );
  });

  it("refreshes the registered opener when the workspace root changes", async () => {
    lspMocks.setFileOpener.mockImplementation((opener) => {
      lspMocks.currentOpener = opener;
    });
    lspMocks.getFileOpener.mockImplementation(() => lspMocks.currentOpener);

    const view = renderHook(() => useLspFileOpener());
    const firstOpener = lspMocks.currentOpener;
    lspMocks.state.taskSessions.items["session-1"].workspace_path = NEXT_TASK_ROOT;
    view.rerender();
    const secondOpener = lspMocks.currentOpener;

    expect(secondOpener).not.toBe(firstOpener);
    await act(async () => {
      await secondOpener?.(
        modelUriForDocument(`file://${NEXT_TASK_ROOT}/${SOURCE_FILE_PATH}`, lspMocks.sessionId),
      );
    });
    expect(lspMocks.openFile).toHaveBeenCalledWith(SOURCE_FILE_PATH, undefined);
  });
});
