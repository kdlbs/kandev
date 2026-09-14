import { act, renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it } from "vitest";
import { createElement, type ReactNode } from "react";
import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import { useCommentsStore, type AgentMessageComment } from "@/lib/state/slices/comments";
import type { TaskPlan } from "@/lib/types/http";
import { usePendingAgentMessageComments, usePendingPlanComments } from "./use-pending-comments";

const PLAN_TIMESTAMP = "2026-09-02T00:00:00Z";
const TASK_ID = "task-1";
const PLAN_ID = "plan-1";

const plan: TaskPlan = {
  id: PLAN_ID,
  task_id: TASK_ID,
  title: "Plan",
  content: "# Plan",
  created_by: "agent",
  created_at: PLAN_TIMESTAMP,
  updated_at: PLAN_TIMESTAMP,
};

function wrapper({ children }: { children: ReactNode }) {
  return createElement(StateProvider, null, children);
}

function messageComment(id: string, sessionId: string): AgentMessageComment {
  return {
    id,
    sessionId,
    source: "agent-message",
    messageId: `reply-${id}`,
    selectedText: "selected reply",
    anchor: {
      messageId: `reply-${id}`,
      start: 0,
      end: 14,
      selectedText: "selected reply",
      prefix: "",
      suffix: "",
    },
    text: "Please clarify this.",
    createdAt: "2026-07-20T00:00:00Z",
    status: "pending",
  };
}

describe("usePendingAgentMessageComments", () => {
  beforeEach(() => {
    useCommentsStore.setState({
      byId: {},
      bySession: {},
      pendingForChat: [],
      editingCommentId: null,
    });
  });

  it("returns only comments for resolved session", () => {
    act(() => {
      useCommentsStore.getState().addComment(messageComment("one", "session-1"));
      useCommentsStore.getState().addComment(messageComment("two", "session-2"));
    });

    const { result } = renderHook(() => usePendingAgentMessageComments("session-1"));

    expect(result.current.map((comment) => comment.id)).toEqual(["one"]);
  });

  it("returns no comments when there is no resolved session", () => {
    act(() => {
      useCommentsStore.getState().addComment(messageComment("one", "session-1"));
    });

    const { result } = renderHook(() => usePendingAgentMessageComments(null));

    expect(result.current).toEqual([]);
  });
});

describe("usePendingPlanComments", () => {
  it("projects one task snapshot for every session composer", () => {
    const { result } = renderHook(
      () => {
        const store = useAppStoreApi();
        return {
          store,
          primary: usePendingPlanComments(TASK_ID),
          secondary: usePendingPlanComments(TASK_ID),
        };
      },
      { wrapper },
    );

    act(() => {
      result.current.store.getState().setTaskPlan(TASK_ID, plan);
      result.current.store.getState().setTaskPlanComments(TASK_ID, {
        task_id: TASK_ID,
        plan_id: PLAN_ID,
        revision: 1,
        comments: [
          {
            id: "comment-1",
            task_id: TASK_ID,
            plan_id: PLAN_ID,
            body: "Shared feedback",
            selected_text: "Plan step",
            anchor_from: 1,
            anchor_to: 10,
            version: 1,
            created_at: PLAN_TIMESTAMP,
            updated_at: PLAN_TIMESTAMP,
          },
        ],
      });
    });

    expect(result.current.primary.map((comment) => comment.id)).toEqual(["comment-1"]);
    expect(result.current.secondary).toEqual(result.current.primary);
  });
});
