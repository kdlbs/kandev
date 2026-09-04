import { act, renderHook, waitFor } from "@testing-library/react";
import type { PlanComment } from "@/lib/state/slices/comments";
import { describe, expect, it, vi } from "vitest";
import { usePlanSelection } from "./use-plan-selection";

const TASK_ID = "task-1";
const OTHER_TASK_ID = "task-2";

const primaryComment: PlanComment = {
  id: "primary-comment",
  sessionId: "",
  taskId: TASK_ID,
  planId: "plan-1",
  version: 1,
  source: "plan",
  text: "Keep this feedback",
  selectedText: "Primary",
  from: 1,
  to: 8,
  createdAt: "2026-09-02T00:00:00.000Z",
  status: "pending",
};

describe("plan selection task ownership", () => {
  it("keeps comment editing state when only the selected session changes", async () => {
    const setEditingCommentId = vi.fn();
    const view = renderHook(
      ({ taskId, sessionId, comments }) => {
        void sessionId;
        return usePlanSelection(taskId, "plan-1", { comments, setEditingCommentId });
      },
      {
        initialProps: {
          taskId: TASK_ID,
          sessionId: "session-primary",
          comments: [primaryComment],
        },
      },
    );

    act(() => {
      view.result.current.handleCommentHighlightClick(primaryComment.id, { x: 20, y: 20 });
    });
    expect(view.result.current.textSelection?.text).toBe(primaryComment.selectedText);
    setEditingCommentId.mockClear();

    view.rerender({ taskId: TASK_ID, sessionId: "session-secondary", comments: [primaryComment] });

    expect(view.result.current.textSelection?.text).toBe(primaryComment.selectedText);
    expect(setEditingCommentId).not.toHaveBeenCalled();

    view.rerender({ taskId: OTHER_TASK_ID, sessionId: "session-other", comments: [] });

    await waitFor(() => expect(view.result.current.textSelection).toBeNull());
    expect(setEditingCommentId).toHaveBeenLastCalledWith(null);
  });

  it("clears comment editing state when the current plan is replaced", async () => {
    const setEditingCommentId = vi.fn();
    const view = renderHook(
      ({ planId }) =>
        usePlanSelection(TASK_ID, planId, {
          comments: [primaryComment],
          setEditingCommentId,
        }),
      { initialProps: { planId: "plan-1" } },
    );
    act(() => {
      view.result.current.handleCommentHighlightClick(primaryComment.id, { x: 20, y: 20 });
    });
    setEditingCommentId.mockClear();

    view.rerender({ planId: "plan-2" });

    await waitFor(() => expect(view.result.current.textSelection).toBeNull());
    expect(setEditingCommentId).toHaveBeenLastCalledWith(null);
  });
});
