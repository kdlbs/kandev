import type { ReactNode } from "react";
import type { DiffLineAnnotation } from "@pierre/diffs";
import { cleanup, fireEvent, render, screen, waitFor, act } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { DiffViewer } from "./diff-viewer";
import type { FileDiffData, DiffComment } from "@/lib/diff/types";
import { useCommentsStore } from "@/lib/state/slices/comments";
import type { AnnotationMetadata } from "./use-diff-annotation-renderer";

type CapturedProps = {
  lineAnnotations?: Array<DiffLineAnnotation<AnnotationMetadata>>;
  renderAnnotation?: (annotation: DiffLineAnnotation<AnnotationMetadata>) => ReactNode;
};

const captured: CapturedProps[] = [];

vi.mock("@/components/theme/app-theme", () => ({
  useTheme: () => ({ resolvedTheme: "dark" }),
}));

// Capture the props (notably lineAnnotations) handed to Pierre's FileDiff so we
// can assert the internal comment-edit pipeline reaches the renderer with the
// updated text. Pierre itself is a web-component-backed renderer that does not
// run meaningfully in jsdom, so stubbing it and inspecting props is the
// reliable way to test propagation.
vi.mock("@pierre/diffs/react", () => ({
  FileDiff: (props: CapturedProps) => {
    captured.push(props);
    return (
      <div data-testid="file-diff">
        {props.lineAnnotations?.map((annotation, index) => (
          <div key={`${annotation.metadata.type}-${index}`}>
            {props.renderAnnotation?.(annotation)}
          </div>
        ))}
      </div>
    );
  },
}));

const SESSION_ID = "session-edit-test";
const FILE_PATH = "src/example.ts";
const COMMENT_ID = "comment-1";
const ORIGINAL_TEXT = "original text";
const EDITED_TEXT = "edited text";

const data: FileDiffData = {
  filePath: FILE_PATH,
  diff: "diff --git a/src/example.ts b/src/example.ts\n--- a/src/example.ts\n+++ b/src/example.ts\n@@ -1 +1 @@\n-old\n+new\n",
  oldContent: "old\n",
  newContent: "new\n",
  additions: 1,
  deletions: 1,
};

function comment(text: string): DiffComment {
  return {
    id: COMMENT_ID,
    sessionId: SESSION_ID,
    source: "diff",
    filePath: FILE_PATH,
    startLine: 1,
    endLine: 1,
    side: "additions",
    codeContent: "new",
    text,
    createdAt: "2026-01-01T00:00:00Z",
    status: "pending",
  };
}

/** Latest comment annotation text handed to Pierre for the target comment. */
function latestCommentText(): string | undefined {
  for (let i = captured.length - 1; i >= 0; i--) {
    const annotations = captured[i].lineAnnotations ?? [];
    const match = annotations.find((a) => a.metadata.comment?.id === COMMENT_ID);
    if (match) return match.metadata.comment?.text;
  }
  return undefined;
}

beforeEach(() => {
  useCommentsStore.setState({
    byId: {},
    bySession: {},
    pendingForChat: [],
    editingCommentId: null,
  });
  captured.length = 0;
});

afterEach(cleanup);

describe("DiffViewer comment editing (internal store mode)", () => {
  it("propagates an edited comment's new text to the renderer", async () => {
    useCommentsStore.getState().addComment(comment(ORIGINAL_TEXT));

    render(<DiffViewer data={data} sessionId={SESSION_ID} enableComments />);

    await waitFor(() => expect(latestCommentText()).toBe(ORIGINAL_TEXT));

    act(() => {
      useCommentsStore.getState().updateComment(COMMENT_ID, { text: EDITED_TEXT });
    });

    await waitFor(() => expect(latestCommentText()).toBe(EDITED_TEXT));
  });

  it("submits edited text from the inline comment form", async () => {
    useCommentsStore.getState().addComment(comment(ORIGINAL_TEXT));

    render(<DiffViewer data={data} sessionId={SESSION_ID} enableComments />);

    await waitFor(() => expect(screen.getByText(ORIGINAL_TEXT)).toBeTruthy());

    fireEvent.click(screen.getByLabelText("Edit comment"));
    fireEvent.change(screen.getByPlaceholderText("Add a comment..."), {
      target: { value: EDITED_TEXT },
    });
    fireEvent.click(screen.getByRole("button", { name: /Update/i }));

    await waitFor(() => expect(latestCommentText()).toBe(EDITED_TEXT));
    expect(screen.getByText(EDITED_TEXT)).toBeTruthy();
  });
});
