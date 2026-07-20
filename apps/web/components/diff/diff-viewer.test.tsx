import type { ReactElement } from "react";
import { cleanup, render, waitFor } from "@testing-library/react";
import { QueryClientProvider } from "@tanstack/react-query";
import { afterEach, describe, expect, it, vi } from "vitest";

import { DiffViewer } from "./diff-viewer";
import type { FileDiffData } from "@/lib/diff/types";
import { makeQueryClient } from "@/lib/query/client";

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

function renderWithQuery(ui: ReactElement) {
  const queryClient = makeQueryClient();
  return render(ui, {
    wrapper: ({ children }) => (
      <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
    ),
  });
}

describe("DiffViewer", () => {
  it("wraps diff lines by default", async () => {
    renderWithQuery(<DiffViewer data={data} />);

    await waitFor(() => expect(fileDiffProps.length).toBeGreaterThan(0));
    expect(fileDiffProps.at(-1)?.options?.overflow).toBe("wrap");
  });

  it("rerenders when the controlled delete handler changes", async () => {
    const firstDelete = vi.fn();
    const secondDelete = vi.fn();
    const { rerender } = renderWithQuery(<DiffViewer data={data} onCommentDelete={firstDelete} />);

    await waitFor(() => expect(fileDiffProps.length).toBeGreaterThan(0));
    const renderCount = fileDiffProps.length;

    rerender(<DiffViewer data={data} onCommentDelete={secondDelete} />);

    await waitFor(() => expect(fileDiffProps.length).toBeGreaterThan(renderCount));
  });

  it("rerenders when the controlled update handler changes", async () => {
    const firstUpdate = vi.fn();
    const secondUpdate = vi.fn();
    const { rerender } = renderWithQuery(<DiffViewer data={data} onCommentUpdate={firstUpdate} />);

    await waitFor(() => expect(fileDiffProps.length).toBeGreaterThan(0));
    const renderCount = fileDiffProps.length;

    rerender(<DiffViewer data={data} onCommentUpdate={secondUpdate} />);

    await waitFor(() => expect(fileDiffProps.length).toBeGreaterThan(renderCount));
  });
});
