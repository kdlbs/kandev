import { describe, it, expect, vi, beforeEach } from "vitest";
import { renderHook, act } from "@testing-library/react";
import { useRef } from "react";
import type { FileEditorState } from "@/lib/state/dockview-store";

const mockUpdateFileContent = vi.fn();
const mockDeleteFile = vi.fn();
const mockGetWebSocketClient = vi.fn();
let openFilesMap = new Map<string, FileEditorState>();

vi.mock("@/lib/ws/workspace-files", () => ({
  updateFileContent: (...args: unknown[]) => mockUpdateFileContent(...args),
  deleteFile: (...args: unknown[]) => mockDeleteFile(...args),
}));

vi.mock("@/lib/ws/connection", () => ({
  getWebSocketClient: () => mockGetWebSocketClient(),
}));

vi.mock("@/lib/state/dockview-store", () => ({
  useDockviewStore: {
    getState: () => ({ openFiles: openFilesMap, api: null }),
  },
}));

vi.mock("@/lib/utils/file-diff", () => ({
  generateUnifiedDiff: () => "@@ -1 +1 @@\n-v1\n+v2",
  calculateHash: async (s: string) => `h:${s.length}`,
}));

import { useSaveDeleteActions, type SaveDeleteParams } from "./use-file-save-delete";

const FAKE_CLIENT = {} as ReturnType<typeof import("@/lib/ws/connection").getWebSocketClient>;
const SESSION_ID = "sess-1";
const PATH = "src/foo.ts";
const REPO = "enrichment-commons";

function seedOpenFile(state: Partial<FileEditorState> = {}) {
  const key = state.repo ? `${state.repo}:${PATH}` : PATH;
  openFilesMap = new Map<string, FileEditorState>([
    [
      key,
      {
        path: PATH,
        name: "foo.ts",
        content: "v2",
        originalContent: "v1",
        originalHash: "h:2",
        isDirty: true,
        ...state,
      },
    ],
  ]);
}

function renderActions() {
  return renderHook(() => {
    const activeSessionIdRef = useRef<string | null>(SESSION_ID);
    const params: SaveDeleteParams = {
      activeSessionIdRef,
      updateFileState: vi.fn(),
      setSavingFiles: vi.fn(),
      toast: vi.fn(),
    };
    return useSaveDeleteActions(params);
  });
}

describe("useSaveDeleteActions repo threading", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockGetWebSocketClient.mockReturnValue(FAKE_CLIENT);
  });

  it("saveFile forwards the file's repo so the diff applies under the right repository", async () => {
    // Multi-repo task: editing foo.ts inside "enrichment-commons" and saving
    // must scope the update to that repo, else the backend writes against the
    // bare task root and the save fails with "file not found".
    seedOpenFile({ repo: REPO });
    mockUpdateFileContent.mockResolvedValueOnce({ success: true, new_hash: "h:2" });

    const { result } = renderActions();
    await act(async () => {
      await result.current.saveFile(PATH, REPO);
    });

    expect(mockUpdateFileContent).toHaveBeenCalledWith(
      FAKE_CLIENT,
      SESSION_ID,
      expect.objectContaining({ path: PATH, repo: REPO }),
    );
  });

  it("deleteFile forwards the file's repo", async () => {
    seedOpenFile({ repo: REPO });
    mockDeleteFile.mockResolvedValueOnce({ success: true });

    const { result } = renderActions();
    await act(async () => {
      await result.current.deleteFileAction(PATH, REPO);
    });

    expect(mockDeleteFile).toHaveBeenCalledWith(FAKE_CLIENT, SESSION_ID, PATH, REPO);
  });
});
