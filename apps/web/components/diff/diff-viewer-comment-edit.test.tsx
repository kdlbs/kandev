import type { ReactElement, ReactNode } from "react";
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
  options?: {
    onLineSelectionEnd?: (range: { side: "additions"; start: number; end: number }) => void;
  };
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
        <button
          type="button"
          data-testid="select-comment-line"
          onClick={() =>
            props.options?.onLineSelectionEnd?.({ side: "additions", start: 1, end: 1 })
          }
        />
        <button
          type="button"
          data-testid="select-another-comment-line"
          onClick={() =>
            props.options?.onLineSelectionEnd?.({ side: "additions", start: 9, end: 9 })
          }
        />
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
const COMMENT_PLACEHOLDER = "Add a comment...";
const DRAFT_COMMENT_TEXT = "draft comment";

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

function latestNewCommentFormProps(): Record<string, unknown> | undefined {
  const props = captured.at(-1);
  const annotation = props?.lineAnnotations?.find(
    (candidate) => candidate.metadata.type === "new-comment-form",
  );
  if (!annotation || !props?.renderAnnotation) return undefined;
  const wrapper = props.renderAnnotation(annotation) as ReactElement<{ children?: ReactNode }>;
  return (wrapper.props.children as ReactElement<Record<string, unknown>>).props;
}

beforeEach(() => {
  window.sessionStorage.clear();
  useCommentsStore.setState({
    byId: {},
    bySession: {},
    pendingForChat: [],
    editingCommentId: null,
  });
  captured.length = 0;
});

afterEach(cleanup);

describe("DiffViewer comment editing", () => {
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
    fireEvent.change(screen.getByPlaceholderText(COMMENT_PLACEHOLDER), {
      target: { value: EDITED_TEXT },
    });
    fireEvent.click(screen.getByRole("button", { name: /Update/i }));

    await waitFor(() => expect(latestCommentText()).toBe(EDITED_TEXT));
    expect(screen.getByText(EDITED_TEXT)).toBeTruthy();
  });

  it("routes controlled inline edits to onCommentUpdate", async () => {
    const onCommentUpdate = vi.fn();

    render(
      <DiffViewer
        data={data}
        sessionId={SESSION_ID}
        enableComments
        comments={[comment(ORIGINAL_TEXT)]}
        onCommentUpdate={onCommentUpdate}
      />,
    );

    await waitFor(() => expect(screen.getByText(ORIGINAL_TEXT)).toBeTruthy());

    fireEvent.click(screen.getByLabelText("Edit comment"));
    fireEvent.change(screen.getByPlaceholderText(COMMENT_PLACEHOLDER), {
      target: { value: EDITED_TEXT },
    });
    fireEvent.click(screen.getByRole("button", { name: /Update/i }));

    await waitFor(() =>
      expect(onCommentUpdate).toHaveBeenCalledWith(COMMENT_ID, { text: EDITED_TEXT }),
    );
    expect(useCommentsStore.getState().byId[COMMENT_ID]?.text).not.toBe(EDITED_TEXT);
  });
});

describe("DiffViewer comment readiness", () => {
  it("keeps a new-comment draft open but rejects direct save and run while comments are disabled", async () => {
    const onCommentAdd = vi.fn();
    const onCommentRun = vi.fn();
    const view = render(
      <DiffViewer
        data={data}
        sessionId={SESSION_ID}
        enableComments
        comments={[]}
        onCommentAdd={onCommentAdd}
        onCommentRun={onCommentRun}
      />,
    );

    fireEvent.click(screen.getByTestId("select-comment-line"));
    const textarea = await screen.findByPlaceholderText(COMMENT_PLACEHOLDER);
    fireEvent.change(textarea, { target: { value: DRAFT_COMMENT_TEXT } });

    view.rerender(
      <DiffViewer
        data={data}
        sessionId={SESSION_ID}
        enableComments={false}
        comments={[]}
        onCommentAdd={onCommentAdd}
        onCommentRun={onCommentRun}
      />,
    );

    expect(screen.getByPlaceholderText(COMMENT_PLACEHOLDER)).toHaveProperty(
      "value",
      DRAFT_COMMENT_TEXT,
    );
    expect((screen.getByRole("button", { name: /Add/i }) as HTMLButtonElement).disabled).toBe(true);
    expect((screen.getByRole("button", { name: /Run/i }) as HTMLButtonElement).disabled).toBe(true);

    const staleForm = latestNewCommentFormProps();
    expect(staleForm?.submitDisabled).toBe(true);
    fireEvent.click(screen.getByTestId("select-another-comment-line"));
    act(() => {
      (staleForm?.onSubmit as (content: string) => void)(DRAFT_COMMENT_TEXT);
      (staleForm?.onSubmitAndRun as (content: string) => void)(DRAFT_COMMENT_TEXT);
    });
    expect(onCommentAdd).not.toHaveBeenCalled();
    expect(onCommentRun).not.toHaveBeenCalled();
    expect(latestCommentText()).toBeUndefined();
    expect(screen.getByPlaceholderText(COMMENT_PLACEHOLDER)).toHaveProperty(
      "value",
      DRAFT_COMMENT_TEXT,
    );

    view.rerender(
      <DiffViewer
        data={data}
        sessionId={SESSION_ID}
        enableComments
        comments={[]}
        onCommentAdd={onCommentAdd}
        onCommentRun={onCommentRun}
      />,
    );
    expect(latestCommentText()).toBeUndefined();
    expect((screen.getByRole("button", { name: /Add/i }) as HTMLButtonElement).disabled).toBe(
      false,
    );
    expect((screen.getByRole("button", { name: /Run/i }) as HTMLButtonElement).disabled).toBe(
      false,
    );
    fireEvent.click(screen.getByRole("button", { name: /Run/i }));
    await waitFor(() => expect(onCommentAdd).toHaveBeenCalledOnce());
    expect(onCommentAdd.mock.calls[0][0].startLine).toBe(1);
    expect(onCommentRun).toHaveBeenCalledOnce();
  });

  it("does not open a line-comment draft from a selection while comments are disabled", async () => {
    const view = render(<DiffViewer data={data} sessionId={SESSION_ID} enableComments />);
    view.rerender(<DiffViewer data={data} sessionId={SESSION_ID} enableComments={false} />);

    fireEvent.click(screen.getByTestId("select-comment-line"));

    expect(screen.queryByPlaceholderText(COMMENT_PLACEHOLDER)).toBeNull();
  });
});
