import { describe, expect, it } from "vitest";
import { joinDestination, normalizeUploadSelection } from "./upload-selection";

/** Build a File carrying an optional webkitRelativePath, as a folder pick does. */
function makeFile(name: string, relativePath?: string): File {
  const file = new File(["x"], name);
  if (relativePath !== undefined) {
    Object.defineProperty(file, "webkitRelativePath", { value: relativePath });
  }
  return file;
}

describe("normalizeUploadSelection", () => {
  it("uses bare names for a flat multi-file pick", () => {
    const { entries, skipped } = normalizeUploadSelection([makeFile("a.txt"), makeFile("b.json")]);

    expect(entries.map((e) => e.relativePath)).toEqual(["a.txt", "b.json"]);
    expect(skipped).toEqual([]);
  });

  it("preserves directory structure from a folder pick", () => {
    const { entries } = normalizeUploadSelection([
      makeFile("root.txt", "fixtures/root.txt"),
      makeFile("leaf.txt", "fixtures/deep/nested/leaf.txt"),
    ]);

    expect(entries.map((e) => e.relativePath)).toEqual([
      "fixtures/root.txt",
      "fixtures/deep/nested/leaf.txt",
    ]);
  });

  it("produces the same shape whichever picker was used", () => {
    const flat = normalizeUploadSelection([makeFile("a.txt")]);
    const folder = normalizeUploadSelection([makeFile("a.txt", "a.txt")]);

    expect(flat.entries[0].relativePath).toBe(folder.entries[0].relativePath);
  });

  it("skips a traversing path instead of failing the batch", () => {
    const { entries, skipped } = normalizeUploadSelection([
      makeFile("good.txt"),
      makeFile("evil.txt", "../evil.txt"),
      makeFile("also-good.txt"),
    ]);

    expect(entries.map((e) => e.relativePath)).toEqual(["good.txt", "also-good.txt"]);
    expect(skipped).toEqual(["../evil.txt"]);
  });

  it("skips an absolute path", () => {
    const { entries, skipped } = normalizeUploadSelection([makeFile("p", "/etc/passwd")]);

    expect(entries).toEqual([]);
    expect(skipped).toEqual(["/etc/passwd"]);
  });

  it("normalizes backslashes and redundant segments", () => {
    const { entries } = normalizeUploadSelection([makeFile("x.txt", "dir\\.\\sub//x.txt")]);

    expect(entries[0].relativePath).toBe("dir/sub/x.txt");
  });

  it("skips an interior traversal segment", () => {
    const { entries, skipped } = normalizeUploadSelection([makeFile("x.txt", "dir/../../x.txt")]);

    expect(entries).toEqual([]);
    expect(skipped).toEqual(["dir/../../x.txt"]);
  });
});

describe("joinDestination", () => {
  it("joins a folder and a relative path", () => {
    expect(joinDestination("fixtures", "a.txt")).toBe("fixtures/a.txt");
    expect(joinDestination("fixtures", "deep/a.txt")).toBe("fixtures/deep/a.txt");
  });

  it("treats an empty folder as the workspace root", () => {
    expect(joinDestination("", "a.txt")).toBe("a.txt");
  });

  it("tolerates surrounding slashes on the folder", () => {
    expect(joinDestination("/fixtures/", "a.txt")).toBe("fixtures/a.txt");
  });
});
