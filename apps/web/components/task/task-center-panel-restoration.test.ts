import { act, renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { OpenFileTab } from "@/lib/types/backend";

const mockUpdateFileContent = vi.fn();
const mockGetWebSocketClient = vi.fn();
const mockRequestFileContent = vi.fn();
const mockSaveDocument = vi.fn();
const mockToast = vi.fn();

vi.mock("@/lib/ws/workspace-files", () => ({
  requestFileContent: (...args: unknown[]) => mockRequestFileContent(...args),
  updateFileContent: (...args: unknown[]) => mockUpdateFileContent(...args),
  deleteFile: vi.fn(),
}));

vi.mock("@/lib/ws/connection", () => ({
  getWebSocketClient: () => mockGetWebSocketClient(),
}));

vi.mock("@/lib/lsp/lsp-client-manager", () => ({
  lspClientManager: {
    saveDocument: (...args: unknown[]) => mockSaveDocument(...args),
  },
}));

vi.mock("@/components/toast-provider", () => ({
  useToast: () => ({ toast: mockToast }),
}));

vi.mock("@/lib/utils/file-diff", () => ({
  calculateHash: async (content: string) => `hash:${content.length}`,
  generateUnifiedDiff: () => "@@ -1 +1 @@\n-before\n+after",
}));

import { loadSavedFileTabs, useFileSaveDelete } from "./task-center-panel-restoration";

const SESSION_ID = "session";
const PATH = "src/Main.kt";
const REPO = "backend";
const CLIENT = {} as ReturnType<typeof import("@/lib/ws/connection").getWebSocketClient>;
const OPEN_TAB: OpenFileTab = {
  path: PATH,
  name: "Main.kt",
  repo: REPO,
  content: "after",
  originalContent: "before",
  originalHash: "old-hash",
  isDirty: true,
};

function renderSaveHook(initialTabs: OpenFileTab[] = [OPEN_TAB]) {
  const setOpenFileTabs = vi.fn();
  const hook = renderHook(
    ({ openFileTabs }) =>
      useFileSaveDelete({
        activeSessionId: SESSION_ID,
        openFileTabs,
        setOpenFileTabs,
        setSavingFiles: vi.fn(),
        handleCloseFileTab: vi.fn(),
      }),
    { initialProps: { openFileTabs: initialTabs } },
  );
  return { ...hook, setOpenFileTabs };
}

describe("task center file saves", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockGetWebSocketClient.mockReturnValue(CLIENT);
  });

  it("notifies the language server with the persisted snapshot after a successful save", async () => {
    mockUpdateFileContent.mockResolvedValueOnce({ success: true, new_hash: "new-hash" });
    const { result } = renderSaveHook();

    await act(async () => {
      await result.current.handleFileSave(PATH, REPO);
    });

    expect(mockSaveDocument).toHaveBeenCalledWith(SESSION_ID, PATH, REPO, "after", "after");
  });

  it("preserves edits made while the persisted save is in flight", async () => {
    let resolveSave!: (response: { success: boolean; new_hash: string }) => void;
    mockUpdateFileContent.mockReturnValueOnce(
      new Promise((resolve) => {
        resolveSave = resolve;
      }),
    );
    const view = renderSaveHook();

    let savePromise!: Promise<void>;
    act(() => {
      savePromise = view.result.current.handleFileSave(PATH, REPO);
    });
    const newerTab = { ...OPEN_TAB, content: "newer edit", isDirty: true };
    view.rerender({ openFileTabs: [newerTab] });
    await act(async () => {
      resolveSave({ success: true, new_hash: "new-hash" });
      await savePromise;
    });

    expect(mockSaveDocument).toHaveBeenCalledWith(SESSION_ID, PATH, REPO, "after", "newer edit");
    const update = view.setOpenFileTabs.mock.calls[0]?.[0] as (
      tabs: OpenFileTab[],
    ) => OpenFileTab[];
    expect(update([newerTab])).toEqual([
      expect.objectContaining({
        content: "newer edit",
        originalContent: "after",
        originalHash: "new-hash",
        isDirty: true,
      }),
    ]);
  });

  it("does not notify the language server after a rejected save", async () => {
    mockUpdateFileContent.mockResolvedValueOnce({ success: false, error: "write failed" });
    const { result } = renderSaveHook();

    await act(async () => {
      await result.current.handleFileSave(PATH, REPO);
    });

    expect(mockSaveDocument).not.toHaveBeenCalled();
  });
});

describe("task center file restoration", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockGetWebSocketClient.mockReturnValue(CLIENT);
  });

  it("restores generic rendered preview state for center-panel tabs", async () => {
    mockRequestFileContent.mockResolvedValueOnce({
      path: "README.md",
      content: "# README",
      is_binary: false,
    });

    const tabs = await loadSavedFileTabs("session", [
      { path: "README.md", name: "README.md", renderedPreview: true, pinned: true },
    ]);

    expect(tabs?.[0]).toEqual({
      path: "README.md",
      name: "README.md",
      content: "# README",
      originalContent: "# README",
      originalHash: "hash:8",
      isDirty: false,
      isBinary: false,
      renderedPreview: true,
    });
  });
});
