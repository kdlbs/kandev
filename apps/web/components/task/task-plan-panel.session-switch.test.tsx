import { act, renderHook, waitFor } from "@testing-library/react";
import type { PlanComment } from "@/lib/state/slices/comments";
import { describe, expect, it, vi } from "vitest";
import { usePlanSelection } from "./use-plan-selection";

const PRIMARY_SESSION_ID = "session-primary";
const SECONDARY_SESSION_ID = "session-secondary";

const primaryComment: PlanComment = {
  id: "primary-comment",
  sessionId: PRIMARY_SESSION_ID,
  source: "plan",
  text: "Keep this feedback",
  selectedText: "Primary",
  from: 1,
  to: 8,
  createdAt: "2026-09-02T00:00:00.000Z",
  status: "pending",
};

describe("plan selection session ownership", () => {
  // @covers AC-UI-PLAN-COMMENT-DRAFTS-001.5
  it("closes comment editing state when the active session changes", async () => {
    const setEditingCommentId = vi.fn();
    const view = renderHook(
      ({ sessionId, comments }) => usePlanSelection(sessionId, { comments, setEditingCommentId }),
      {
        initialProps: {
          sessionId: PRIMARY_SESSION_ID,
          comments: [primaryComment],
        },
      },
    );

    act(() => {
      view.result.current.handleCommentHighlightClick(primaryComment.id, { x: 20, y: 20 });
    });
    expect(view.result.current.textSelection?.text).toBe(primaryComment.selectedText);
    setEditingCommentId.mockClear();

    view.rerender({ sessionId: SECONDARY_SESSION_ID, comments: [] });

    await waitFor(() => expect(view.result.current.textSelection).toBeNull());
    expect(setEditingCommentId).toHaveBeenLastCalledWith(null);
  });
});
