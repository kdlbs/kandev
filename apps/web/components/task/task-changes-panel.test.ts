import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import {
  shouldCloseFileDiffPanel,
  filterVisibleFiles,
  scrollToFileAndClear,
} from "./task-changes-panel";
import type { ReviewFile } from "@/components/review/types";

const PATH = "src/foo.ts";

describe("shouldCloseFileDiffPanel", () => {
  it("returns false when gitStatus is undefined (not loaded yet)", () => {
    expect(shouldCloseFileDiffPanel(undefined, PATH)).toBe(false);
  });

  it("returns true when gitStatus.files is undefined (loaded, no changes)", () => {
    expect(shouldCloseFileDiffPanel({}, PATH)).toBe(true);
  });

  it("returns true when the file is missing from gitStatus.files (discarded)", () => {
    expect(shouldCloseFileDiffPanel({ files: {} }, PATH)).toBe(true);
  });

  it("returns false when the file has a non-empty uncommitted diff", () => {
    const gitStatus = { files: { [PATH]: { diff: "@@ -1 +1 @@\n-a\n+b\n" } } };
    expect(shouldCloseFileDiffPanel(gitStatus, PATH)).toBe(false);
  });

  it("returns true when the file entry exists but diff is an empty string", () => {
    const gitStatus = { files: { [PATH]: { diff: "" } } };
    expect(shouldCloseFileDiffPanel(gitStatus, PATH)).toBe(true);
  });

  it("returns true when the file entry exists but diff is undefined", () => {
    const gitStatus = { files: { [PATH]: {} } };
    expect(shouldCloseFileDiffPanel(gitStatus, PATH)).toBe(true);
  });

  it("is not affected by unrelated files in gitStatus.files", () => {
    const gitStatus = { files: { "other/file.ts": { diff: "diff content" } } };
    expect(shouldCloseFileDiffPanel(gitStatus, PATH)).toBe(true);
  });
});

function file(path: string, source: ReviewFile["source"]): ReviewFile {
  return {
    path,
    diff: "@@@@",
    status: "modified",
    additions: 0,
    deletions: 0,
    staged: false,
    source,
  };
}

describe("filterVisibleFiles", () => {
  const files: ReviewFile[] = [
    file("a.ts", "uncommitted"),
    file("b.ts", "committed"),
    file("c.ts", "pr"),
  ];

  it("returns all files when mode=all and sourceFilter=all", () => {
    expect(filterVisibleFiles(files, "all", undefined, undefined, "all")).toHaveLength(3);
  });

  it("filters by uncommitted source", () => {
    const result = filterVisibleFiles(files, "all", undefined, undefined, "uncommitted");
    expect(result).toHaveLength(1);
    expect(result[0].path).toBe("a.ts");
  });

  it("filters by pr source", () => {
    const result = filterVisibleFiles(files, "all", undefined, undefined, "pr");
    expect(result).toHaveLength(1);
    expect(result[0].path).toBe("c.ts");
  });

  it("filters by committed source", () => {
    const result = filterVisibleFiles(files, "all", undefined, undefined, "committed");
    expect(result).toHaveLength(1);
    expect(result[0].path).toBe("b.ts");
  });

  it("file-mode + sourceFilter intersect (file present in source)", () => {
    const result = filterVisibleFiles(files, "file", "a.ts", undefined, "uncommitted");
    expect(result).toHaveLength(1);
    expect(result[0].path).toBe("a.ts");
  });

  it("file-mode + sourceFilter intersect (file absent from source)", () => {
    const result = filterVisibleFiles(files, "file", "a.ts", undefined, "pr");
    expect(result).toHaveLength(0);
  });

  it("returns empty list when no files match", () => {
    expect(filterVisibleFiles([], "all", undefined, undefined, "all")).toEqual([]);
  });

  it("file-mode filters by repository name when provided", () => {
    const samePathMultiRepo: ReviewFile[] = [
      { ...file("README.md", "uncommitted"), repository_name: "frontend" },
      { ...file("README.md", "uncommitted"), repository_name: "backend" },
    ];
    const result = filterVisibleFiles(
      samePathMultiRepo,
      "file",
      "README.md",
      "frontend",
      "uncommitted",
    );
    expect(result).toHaveLength(1);
    expect(result[0].repository_name).toBe("frontend");
  });
});

describe("scrollToFileAndClear", () => {
  const rafCallbacks: Array<FrameRequestCallback> = [];

  beforeEach(() => {
    vi.stubGlobal("requestAnimationFrame", (cb: FrameRequestCallback) => {
      rafCallbacks.push(cb);
      return 0;
    });
  });

  afterEach(() => {
    rafCallbacks.length = 0;
    vi.unstubAllGlobals();
  });

  it("defers onClearSelected into rAF when ref has a DOM element", () => {
    const el = document.createElement("div");
    el.scrollIntoView = vi.fn();
    const ref = { current: el };
    const fileRefs = new Map([["src/pr.ts", ref]]);
    const onClearSelected = vi.fn();

    scrollToFileAndClear("src/pr.ts", fileRefs, onClearSelected);

    expect(onClearSelected).not.toHaveBeenCalled();
    expect(rafCallbacks).toHaveLength(1);

    rafCallbacks[0](0);

    expect(el.scrollIntoView).toHaveBeenCalledWith({ behavior: "smooth", block: "start" });
    expect(onClearSelected).toHaveBeenCalledOnce();
  });

  it("calls onClearSelected immediately when ref has no DOM element", () => {
    const ref = { current: null };
    const fileRefs = new Map([["src/pr.ts", ref]]);
    const onClearSelected = vi.fn();

    scrollToFileAndClear("src/pr.ts", fileRefs, onClearSelected);

    expect(onClearSelected).toHaveBeenCalledOnce();
    expect(rafCallbacks).toHaveLength(0);
  });

  it("calls onClearSelected immediately when path not in fileRefs", () => {
    const fileRefs = new Map<string, { current: HTMLDivElement | null }>();
    const onClearSelected = vi.fn();

    scrollToFileAndClear("src/missing.ts", fileRefs, onClearSelected);

    expect(onClearSelected).toHaveBeenCalledOnce();
    expect(rafCallbacks).toHaveLength(0);
  });
});
