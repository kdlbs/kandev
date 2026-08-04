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

function renderSaveHook() {
  return renderHook(() =>
    useFileSaveDelete({
      activeSessionId: SESSION_ID,
      openFileTabs: [OPEN_TAB],
      setOpenFileTabs: vi.fn(),
      setSavingFiles: vi.fn(),
      handleCloseFileTab: vi.fn(),
    }),
  );
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

    expect(mockSaveDocument).toHaveBeenCalledWith(SESSION_ID, PATH, REPO, "after");
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
