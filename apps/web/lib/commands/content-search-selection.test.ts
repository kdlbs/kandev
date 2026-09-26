import { beforeEach, describe, expect, it, vi } from "vitest";
import type { WorkspaceContentSearchResult } from "@/lib/types/backend";

const mockSetPendingCursorPosition = vi.fn();
const mockScrollEditorIfMounted = vi.fn();
const mockAddFileEditorPanel = vi.fn();

vi.mock("@/hooks/file-editor-cursor", () => ({
  setPendingCursorPosition: (...args: unknown[]) => mockSetPendingCursorPosition(...args),
  scrollEditorIfMounted: (...args: unknown[]) => mockScrollEditorIfMounted(...args),
}));

vi.mock("@/lib/state/dockview-store", () => ({
  useDockviewStore: {
    getState: () => ({ addFileEditorPanel: mockAddFileEditorPanel }),
  },
}));

import { openContentSearchResult } from "./content-search-selection";

const FILE_PATH = "src/components/search.tsx";
const FILE_NAME = "search.tsx";
const SESSION_ID = "session-1";

const result: WorkspaceContentSearchResult = {
  repository_name: "frontend",
  path: FILE_PATH,
  line: 42,
  column: 7,
  preview: "function search()",
  match_ranges: [{ start: 9, end: 15 }],
};

describe("openContentSearchResult", () => {
  beforeEach(() => {
    mockSetPendingCursorPosition.mockReset();
    mockScrollEditorIfMounted.mockReset();
    mockAddFileEditorPanel.mockReset();
  });

  it("opens the matching repository file at its exact cursor position", () => {
    openContentSearchResult(result, "/tasks/task-1", SESSION_ID);

    expect(mockSetPendingCursorPosition).toHaveBeenCalledWith(FILE_PATH, 42, 7, "frontend", {
      sessionId: SESSION_ID,
      flashLine: true,
    });
    expect(mockScrollEditorIfMounted).toHaveBeenCalledWith(FILE_PATH, "/tasks/task-1", 42, 7, {
      repo: "frontend",
      sessionId: SESSION_ID,
      flashLine: true,
    });
    expect(mockAddFileEditorPanel).toHaveBeenCalledWith(FILE_PATH, FILE_NAME, {
      repo: "frontend",
    });
    expect(mockAddFileEditorPanel.mock.invocationCallOrder[0]).toBeLessThan(
      mockScrollEditorIfMounted.mock.invocationCallOrder[0],
    );
  });

  it("omits an empty repository key for a single-repo workspace", () => {
    openContentSearchResult({ ...result, repository_name: "" }, null, SESSION_ID);

    expect(mockSetPendingCursorPosition).toHaveBeenCalledWith(FILE_PATH, 42, 7, undefined, {
      sessionId: SESSION_ID,
      flashLine: true,
    });
    expect(mockScrollEditorIfMounted).toHaveBeenCalledWith(FILE_PATH, null, 42, 7, {
      repo: undefined,
      sessionId: SESSION_ID,
      flashLine: true,
    });
    expect(mockAddFileEditorPanel).toHaveBeenCalledWith(FILE_PATH, FILE_NAME, {
      repo: undefined,
    });
  });
});
