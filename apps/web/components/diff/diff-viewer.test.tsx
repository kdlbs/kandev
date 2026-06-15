import { cleanup, render, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { DiffViewer } from "./diff-viewer";
import type { FileDiffData } from "@/lib/diff/types";

const fileDiffProps: Array<{ options?: { overflow?: string } }> = [];

vi.mock("@/components/theme/app-theme", () => ({
  useTheme: () => ({ resolvedTheme: "dark" }),
}));

vi.mock("@pierre/diffs/react", () => ({
  FileDiff: (props: { options?: { overflow?: string } }) => {
    fileDiffProps.push(props);
    return <div data-testid="file-diff" />;
  },
}));

const data: FileDiffData = {
  filePath: "src/example.ts",
  diff: "diff --git a/src/example.ts b/src/example.ts\n--- a/src/example.ts\n+++ b/src/example.ts\n@@ -1 +1 @@\n-old\n+new\n",
  oldContent: "old\n",
  newContent: "new\n",
  additions: 1,
  deletions: 1,
};

afterEach(() => {
  cleanup();
  fileDiffProps.length = 0;
});

describe("DiffViewer", () => {
  it("wraps diff lines by default", async () => {
    render(<DiffViewer data={data} />);

    await waitFor(() => expect(fileDiffProps.length).toBeGreaterThan(0));
    expect(fileDiffProps.at(-1)?.options?.overflow).toBe("wrap");
  });
});
