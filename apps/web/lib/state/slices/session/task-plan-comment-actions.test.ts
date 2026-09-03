import { describe, expect, it } from "vitest";
import { createAppStore } from "@/lib/state/store";
import type { TaskPlan, TaskPlanCommentSnapshot } from "@/lib/types/http";

const TASK_ID = "task-1";
const PLAN_TIMESTAMP = "2026-09-02T00:00:00Z";

function plan(id = "plan-1"): TaskPlan {
  return {
    id,
    task_id: TASK_ID,
    title: "Plan",
    content: "# Plan",
    created_by: "agent",
    created_at: PLAN_TIMESTAMP,
    updated_at: PLAN_TIMESTAMP,
  };
}

function snapshot(revision: number, planId = "plan-1"): TaskPlanCommentSnapshot {
  return {
    task_id: TASK_ID,
    plan_id: planId,
    revision,
    comments: [
      {
        id: `comment-${revision}`,
        task_id: TASK_ID,
        plan_id: planId,
        body: `feedback ${revision}`,
        selected_text: "selected",
        anchor_from: 1,
        anchor_to: 4,
        version: 1,
        created_at: PLAN_TIMESTAMP,
        updated_at: PLAN_TIMESTAMP,
      },
    ],
  };
}

describe("task plan comment state", () => {
  it("keeps the newest complete snapshot for the current plan", () => {
    const store = createAppStore();
    store.getState().setTaskPlan(TASK_ID, plan());

    store.getState().setTaskPlanComments(TASK_ID, snapshot(2));
    store.getState().setTaskPlanComments(TASK_ID, snapshot(1));

    expect(store.getState().taskPlans.commentsByTaskId[TASK_ID]).toEqual(snapshot(2));
    expect(store.getState().taskPlans.commentsLoadedByTaskId[TASK_ID]).toBe(true);
    expect(store.getState().taskPlans.commentsLoadingByTaskId[TASK_ID]).toBe(false);
  });

  it("rejects an old-plan event and clears comments when plan identity changes", () => {
    const store = createAppStore();
    store.getState().setTaskPlan(TASK_ID, plan());
    store.getState().setTaskPlanComments(TASK_ID, snapshot(2));
    store.getState().setTaskPlanCommentMigrationStatus(TASK_ID, "complete");
    store.getState().setTaskPlanComments(TASK_ID, snapshot(99, "old-plan"));
    expect(store.getState().taskPlans.commentsByTaskId[TASK_ID]).toEqual(snapshot(2));

    store.getState().setTaskPlan(TASK_ID, plan("plan-2"));
    expect(store.getState().taskPlans.commentsByTaskId[TASK_ID]).toBeUndefined();
    expect(store.getState().taskPlans.commentsLoadedByTaskId[TASK_ID]).toBe(false);
    expect(store.getState().taskPlans.commentsMigrationStatusByTaskId[TASK_ID]).toBe("idle");
  });

  it("clears the task snapshot when the plan is deleted", () => {
    const store = createAppStore();
    store.getState().setTaskPlan(TASK_ID, plan());
    store.getState().setTaskPlanComments(TASK_ID, snapshot(1));
    store.getState().setTaskPlanCommentMigrationStatus(TASK_ID, "complete");

    store.getState().setTaskPlan(TASK_ID, null);

    expect(store.getState().taskPlans.commentsByTaskId[TASK_ID]).toBeUndefined();
    expect(store.getState().taskPlans.commentsLoadedByTaskId[TASK_ID]).toBe(true);
    expect(store.getState().taskPlans.commentsMigrationStatusByTaskId[TASK_ID]).toBe("idle");
  });
});
