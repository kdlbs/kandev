import { act, renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { useCommentActions } from "./comment-utils";
import type { DiffComment } from "./types";

const COMMENT_ID = "comment-1";
const EDITED_TEXT = "edited text";

function comment(): DiffComment {
  return {
    id: COMMENT_ID,
    source: "diff",
    sessionId: "session-1",
    filePath: "src/example.ts",
    startLine: 1,
    endLine: 1,
    side: "additions",
    codeContent: "new line",
    text: "original",
    createdAt: "2026-01-01T00:00:00Z",
    status: "pending",
  };
}

afterEach(() => {
  vi.restoreAllMocks();
});

describe("useCommentActions", () => {
  it("routes updates to the external handler in controlled comment mode", () => {
    const removeComment = vi.fn();
    const updateComment = vi.fn();
    const setEditingComment = vi.fn();
    const onCommentUpdate = vi.fn();

    const { result } = renderHook(() =>
      useCommentActions({
        removeComment,
        updateComment,
        setEditingComment,
        onCommentUpdate,
        externalComments: [comment()],
      }),
    );

    act(() => {
      result.current.handleCommentUpdate(COMMENT_ID, EDITED_TEXT);
    });

    expect(onCommentUpdate).toHaveBeenCalledWith(COMMENT_ID, { text: EDITED_TEXT });
    expect(updateComment).not.toHaveBeenCalled();
    expect(setEditingComment).toHaveBeenCalledWith(null);
    expect(removeComment).not.toHaveBeenCalled();
  });

  it("updates the internal store handler when comments are uncontrolled", () => {
    const updateComment = vi.fn();
    const setEditingComment = vi.fn();

    const { result } = renderHook(() =>
      useCommentActions({
        removeComment: vi.fn(),
        updateComment,
        setEditingComment,
      }),
    );

    act(() => {
      result.current.handleCommentUpdate(COMMENT_ID, EDITED_TEXT);
    });

    expect(updateComment).toHaveBeenCalledWith(COMMENT_ID, { text: EDITED_TEXT });
    expect(setEditingComment).toHaveBeenCalledWith(null);
  });

  it("does not write to the internal store when controlled updates lack a handler", () => {
    const updateComment = vi.fn();
    const setEditingComment = vi.fn();
    const warn = vi.spyOn(console, "warn").mockImplementation(() => {});

    const { result } = renderHook(() =>
      useCommentActions({
        removeComment: vi.fn(),
        updateComment,
        setEditingComment,
        externalComments: [comment()],
      }),
    );

    act(() => {
      result.current.handleCommentUpdate(COMMENT_ID, EDITED_TEXT);
    });

    expect(updateComment).not.toHaveBeenCalled();
    expect(warn).toHaveBeenCalledWith(expect.stringContaining("`onCommentUpdate`"));
    expect(setEditingComment).toHaveBeenCalledWith(null);
  });

  it("routes deletes to the external handler in controlled comment mode", () => {
    const removeComment = vi.fn();
    const onCommentDelete = vi.fn();

    const { result } = renderHook(() =>
      useCommentActions({
        removeComment,
        updateComment: vi.fn(),
        setEditingComment: vi.fn(),
        onCommentDelete,
        externalComments: [comment()],
      }),
    );

    act(() => {
      result.current.handleCommentDelete(COMMENT_ID);
    });

    expect(onCommentDelete).toHaveBeenCalledWith(COMMENT_ID);
    expect(removeComment).not.toHaveBeenCalled();
  });

  it("routes deletes to the internal store handler when comments are uncontrolled", () => {
    const removeComment = vi.fn();

    const { result } = renderHook(() =>
      useCommentActions({
        removeComment,
        updateComment: vi.fn(),
        setEditingComment: vi.fn(),
      }),
    );

    act(() => {
      result.current.handleCommentDelete(COMMENT_ID);
    });

    expect(removeComment).toHaveBeenCalledWith(COMMENT_ID);
  });
});
