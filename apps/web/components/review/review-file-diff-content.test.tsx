import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { ReviewFile } from "./types";

const viewer = vi.hoisted(() => ({
  mounts: 0,
  lastProps: undefined as Record<string, unknown> | undefined,
}));

vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));
vi.mock("@/components/diff", async () => {
  const React = await import("react");
  return {
    DiffErrorBoundary: ({ children }: { children: React.ReactNode }) => <>{children}</>,
    FileDiffViewer: (props: Record<string, unknown>) => {
      React.useEffect(() => {
        viewer.mounts += 1;
      }, []);
      viewer.lastProps = props;
      return <div data-testid="file-diff-viewer">{String(props.diff ?? "")}</div>;
    },
  };
});

import { ReviewFileDiffContent } from "./review-file-diff-content";

const readyFile: ReviewFile = {
  path: "src/app.ts",
  diff: "@@ -1 +1 @@\n-old\n+new",
  diff_state: "ready",
  status: "modified",
  additions: 1,
  deletions: 1,
  staged: false,
  source: "uncommitted",
};

function props(file: ReviewFile) {
  return {
    shouldRender: true,
    file,
    sessionId: "session-1",
    wordWrap: false,
    enableWalkthroughAnnotations: false,
    expandUnchanged: false,
    enableExpansion: false,
    baseRef: "HEAD",
    onRevertBlock: vi.fn(),
    onCommentRun: vi.fn(),
    onToggleExpandUnchanged: vi.fn(),
  };
}

afterEach(() => {
  cleanup();
  viewer.mounts = 0;
  viewer.lastProps = undefined;
});

describe("ReviewFileDiffContent refresh continuity", () => {
  it("keeps the viewer mounted through pending, failure, identical, and changed completion", () => {
    const view = render(<ReviewFileDiffContent {...props(readyFile)} />);
    expect(viewer.mounts).toBe(1);

    const pending = { ...readyFile, diff_state: "pending" as const, display_stale: true };
    view.rerender(
      <ReviewFileDiffContent {...props(pending)} enableExpansion enableWalkthroughAnnotations />,
    );
    expect(screen.getByTestId("file-diff-viewer")).toBeTruthy();
    expect(viewer.mounts).toBe(1);
    expect(viewer.lastProps).toMatchObject({ enableComments: false, enableAcceptReject: false });
    expect(viewer.lastProps).toMatchObject({
      enableWalkthroughAnnotations: false,
      enableExpansion: false,
    });
    expect(viewer.lastProps?.onRevertBlock).toBeUndefined();
    expect(screen.getByRole("status").textContent).toContain("task:gitDiffLoading");

    view.rerender(<ReviewFileDiffContent {...props({ ...pending, diff_state: "unavailable" })} />);
    expect(viewer.mounts).toBe(1);
    expect(screen.getByRole("status").textContent).toContain("task:gitDiffUnavailable");

    view.rerender(<ReviewFileDiffContent {...props({ ...readyFile, diff_state: "ready" })} />);
    view.rerender(
      <ReviewFileDiffContent
        {...props({ ...readyFile, diff: "@@ -1 +1 @@\n-old\n+updated", diff_state: "ready" })}
      />,
    );
    expect(viewer.mounts).toBe(1);
    expect(screen.getByTestId("file-diff-viewer").textContent).toContain("updated");
  });

  it("keeps the initial placeholder when there is no prior display", () => {
    render(<ReviewFileDiffContent {...props({ ...readyFile, diff: "", diff_state: "pending" })} />);
    expect(screen.queryByTestId("file-diff-viewer")).toBeNull();
    expect(screen.getByTestId("review-diff-pending")).toBeTruthy();
  });
});
