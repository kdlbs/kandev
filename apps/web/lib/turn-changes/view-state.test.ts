import { beforeEach, describe, expect, it } from "vitest";
import { readTurnChangeViewState, writeTurnChangeViewState } from "./view-state";

describe("turn change view state", () => {
  beforeEach(() => localStorage.clear());

  it("persists expanded folders, selected file, and whitespace choice by session and turn", () => {
    writeTurnChangeViewState("session/one", "change-set", {
      expandedKeys: ["repo", "repo:src"],
      selectedFileId: "file-2",
      selectedFilePath: "src/file.ts",
      selectedFileKind: "renamed",
      selectedFileOldPath: "src/old.ts",
      selectedRepositoryChangeId: "repository-change-1",
      selectedCheckoutId: "checkout-1",
      ignoreWhitespace: true,
    });

    expect(readTurnChangeViewState("session/one", "change-set")).toEqual({
      expandedKeys: ["repo", "repo:src"],
      selectedFileId: "file-2",
      selectedFilePath: "src/file.ts",
      selectedFileKind: "renamed",
      selectedFileOldPath: "src/old.ts",
      selectedRepositoryChangeId: "repository-change-1",
      selectedCheckoutId: "checkout-1",
      ignoreWhitespace: true,
    });
    expect(readTurnChangeViewState("session/other", "change-set")).toBeNull();
    expect(readTurnChangeViewState("session/one", "later-turn")).toBeNull();
  });

  it("discards malformed storage and caps expansion state", () => {
    localStorage.setItem("kandev:turn-change-view:session:change", "not-json");
    expect(readTurnChangeViewState("session", "change")).toBeNull();

    writeTurnChangeViewState("session", "bounded", {
      expandedKeys: Array.from({ length: 700 }, (_, index) => String(index)),
      ignoreWhitespace: false,
    });
    expect(readTurnChangeViewState("session", "bounded")?.expandedKeys).toHaveLength(500);
  });
});
