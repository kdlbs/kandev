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
    openContentSearchResult(result, "/tasks/task-1");

    expect(mockSetPendingCursorPosition).toHaveBeenCalledWith(FILE_PATH, 42, 7, "frontend");
    expect(mockScrollEditorIfMounted).toHaveBeenCalledWith(
      FILE_PATH,
      "/tasks/task-1",
      42,
      7,
      "frontend",
    );
    expect(mockAddFileEditorPanel).toHaveBeenCalledWith(FILE_PATH, FILE_NAME, {
      repo: "frontend",
    });
  });

  it("omits an empty repository key for a single-repo workspace", () => {
    openContentSearchResult({ ...result, repository_name: "" }, null);

    expect(mockSetPendingCursorPosition).toHaveBeenCalledWith(FILE_PATH, 42, 7, undefined);
    expect(mockAddFileEditorPanel).toHaveBeenCalledWith(FILE_PATH, FILE_NAME, {
      repo: undefined,
    });
  });
});
