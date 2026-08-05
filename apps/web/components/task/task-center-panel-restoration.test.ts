import { act, renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { OpenFileTab } from "@/lib/types/backend";

const mockUpdateFileContent = vi.fn();
const mockGetWebSocketClient = vi.fn();
const mockSaveDocument = vi.fn();
const mockToast = vi.fn();

vi.mock("@/lib/ws/workspace-files", () => ({
  requestFileContent: vi.fn(),
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
  calculateHash: vi.fn(),
  generateUnifiedDiff: () => "@@ -1 +1 @@\n-before\n+after",
}));

import { useFileSaveDelete } from "./task-center-panel-restoration";

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
