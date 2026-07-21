import { describe, expect, it } from "vitest";
import {
  fileChangeStatusLabel,
  normalizeFileChangeStatus,
  type FileChangeStatus,
} from "./file-change-status";

describe("normalizeFileChangeStatus", () => {
  it.each<FileChangeStatus>(["added", "modified", "deleted", "untracked", "renamed"])(
    "preserves local status %s",
    (status) => {
      expect(normalizeFileChangeStatus(status)).toBe(status);
    },
  );

  it("maps GitHub removed status to deleted", () => {
    expect(normalizeFileChangeStatus("removed")).toBe("deleted");
  });

  it.each([undefined, "copied", "changed", "unexpected"])(
    "falls back to modified for %s",
    (status) => {
      expect(normalizeFileChangeStatus(status)).toBe("modified");
    },
  );
});

describe("fileChangeStatusLabel", () => {
  it.each<[FileChangeStatus, string]>([
    ["added", "Added"],
    ["untracked", "Untracked"],
    ["modified", "Modified"],
    ["deleted", "Deleted"],
    ["renamed", "Moved"],
  ])("labels %s as %s", (status, label) => {
    expect(fileChangeStatusLabel(status)).toBe(label);
  });

  it("includes previous path for moved files", () => {
    expect(fileChangeStatusLabel("renamed", "src/old-name.ts")).toBe("Moved from src/old-name.ts");
  });
});
