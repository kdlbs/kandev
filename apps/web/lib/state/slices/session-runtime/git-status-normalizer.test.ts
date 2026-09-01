import { describe, expect, it } from "vitest";
import { normalizeGitStatusEntry, normalizeGitStatusFiles } from "./git-status-normalizer";
import type { FileInfo, GitStatusEntry } from "./types";

function file(overrides: Partial<FileInfo> = {}): FileInfo {
  return { path: "src/app.ts", status: "modified", staged: false, ...overrides };
}

describe("normalizeGitStatusFiles", () => {
  it("fills a missing path from a legacy map key", () => {
    const files = { "src/app.ts": { status: "modified", staged: false } as FileInfo };

    expect(normalizeGitStatusFiles(files)).toEqual({
      "src/app.ts": file(),
    });
  });

  it("fills both path and repository from a composite map key", () => {
    const files = {
      "frontend\u0000src/app.ts": { status: "modified", staged: false } as FileInfo,
      "backend\u0000src/app.ts": { status: "added", staged: false } as FileInfo,
    };

    expect(normalizeGitStatusFiles(files)).toEqual({
      "frontend\u0000src/app.ts": {
        path: "src/app.ts",
        repository_name: "frontend",
        status: "modified",
        staged: false,
      },
      "backend\u0000src/app.ts": {
        path: "src/app.ts",
        repository_name: "backend",
        status: "added",
        staged: false,
      },
    });
  });

  it("preserves explicit values, including an explicit root repository", () => {
    const files = {
      "frontend\u0000src/app.ts": file({ path: "renamed.ts", repository_name: "other" }),
      "\u0000README.md": file({ path: "README.md", repository_name: "" }),
    };

    expect(normalizeGitStatusFiles(files)).toBe(files);
  });
});

describe("normalizeGitStatusEntry", () => {
  it("returns the same entry when all file records already use the current shape", () => {
    const entry = { files: { "src/app.ts": file() } } as unknown as GitStatusEntry;

    expect(normalizeGitStatusEntry(entry)).toBe(entry);
  });

  it("normalizes a missing file map to an empty object", () => {
    const entry = { files: undefined } as unknown as GitStatusEntry;

    expect(normalizeGitStatusEntry(entry).files).toEqual({});
  });
});
