import { describe, it, expect } from "vitest";
import { renderHook } from "@testing-library/react";

import { useDiffMetadata } from "./use-diff-metadata";
import type { FileDiffData } from "@/lib/diff/types";

const PATCH = `diff --git a/src/foo.ts b/src/foo.ts
--- a/src/foo.ts
+++ b/src/foo.ts
@@ -1,3 +1,3 @@
 first
-second
+SECOND
 third
`;

const baseData = (overrides: Partial<FileDiffData> = {}): FileDiffData => ({
  filePath: "src/foo.ts",
  oldContent: "",
  newContent: "",
  additions: 0,
  deletions: 0,
  ...overrides,
});

describe("useDiffMetadata", () => {
  it("returns null when there is no diff or content", () => {
    const { result } = renderHook(() => useDiffMetadata(baseData()));
    expect(result.current).toBeNull();
  });

  it("parses a unified diff string via parsePatchFiles", () => {
    const { result } = renderHook(() => useDiffMetadata(baseData({ diff: PATCH })));
    expect(result.current).not.toBeNull();
    // Surface checks on the parsed metadata. We do not assert deep internals —
    // we only confirm the library still returns the shape useDiffMetadata
    // depends on (a FileDiffMetadata with at least a recognisable filename).
    const meta = result.current as unknown as Record<string, unknown>;
    const filename = (meta.filename ?? meta.name ?? meta.path) as string | undefined;
    expect(filename).toBeTruthy();
    expect(String(filename)).toContain("foo");
  });

  it("parses from oldContent/newContent when diff is absent", () => {
    const { result } = renderHook(() =>
      useDiffMetadata(
        baseData({
          oldContent: "first\nsecond\nthird\n",
          newContent: "first\nSECOND\nthird\n",
        }),
      ),
    );
    expect(result.current).not.toBeNull();
  });

  it("forces lang='text' on Go files with the problematic regex pattern", () => {
    const goContent = 'package main\n\nvar s interface{} `json:"x"`\n';
    const { result } = renderHook(() =>
      useDiffMetadata(
        baseData({
          filePath: "src/foo.go",
          oldContent: "package main\n",
          newContent: goContent,
        }),
      ),
    );
    expect(result.current).not.toBeNull();
    expect((result.current as { lang?: string }).lang).toBe("text");
  });

  it("leaves lang untouched for Go files without the problematic pattern", () => {
    const { result } = renderHook(() =>
      useDiffMetadata(
        baseData({
          filePath: "src/foo.go",
          oldContent: "package main\n",
          newContent: "package main\n\nfunc main() {}\n",
        }),
      ),
    );
    expect(result.current).not.toBeNull();
    expect((result.current as { lang?: string }).lang).not.toBe("text");
  });

  it("memoizes the result across re-renders with the same inputs", () => {
    const data = baseData({ diff: PATCH });
    const { result, rerender } = renderHook(({ d }) => useDiffMetadata(d), {
      initialProps: { d: data },
    });
    const first = result.current;
    rerender({ d: { ...data } });
    expect(result.current).toBe(first);
  });
});
