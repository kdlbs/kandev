import { describe, it, expect } from "vitest";
import {
  classify,
  type AttentionTask,
  type AttentionStall,
  type AttentionProposal,
} from "./attention";

const T_08 = "2026-09-27T08:00:00Z";
const T_09 = "2026-09-27T09:00:00Z";
const T_10 = "2026-09-27T10:00:00Z";
const T_11 = "2026-09-27T11:00:00Z";
const NOW = new Date("2026-09-27T12:00:00Z");
const NOW_MS = NOW.getTime();

function task(overrides: Partial<AttentionTask> & { id: string }): AttentionTask {
  return {
    title: "Task",
    updatedAt: T_10,
    ...overrides,
  };
}

function stall(overrides: Partial<AttentionStall> & { task_id: string }): AttentionStall {
  return {
    stalled_for_ms: 3_600_000,
    last_event_at: T_09,
    detected_at: T_10,
    ...overrides,
  };
}

function proposal(overrides: Partial<AttentionProposal> & { id: string }): AttentionProposal {
  return {
    status: "pending",
    task_id: null,
    created_at: T_08,
    spec: {
      title: "New task",
      description: "",
      rationale: "",
      workflow_id: "wf-1",
      step_id: "step-1",
      repository_id: "repo-1",
      source_task_id: "",
    },
    ...overrides,
  };
}

describe("classify - basic classification", () => {
  it("classifies a task with pending_action clarification as a question", () => {
    const t = task({
      id: "t-1",
      statusSummary: { pending_action: "clarification", last_activity_at: T_11 },
    });
    const result = classify([t], [], [], NOW);
    expect(result.needsYou).toHaveLength(1);
    expect(result.needsYou[0]).toMatchObject({ kind: "question", pendingAction: "clarification" });
  });

  it("classifies a task with pending_action permission as a question", () => {
    const t = task({
      id: "t-1",
      statusSummary: { pending_action: "permission", last_activity_at: T_11 },
    });
    const result = classify([t], [], [], NOW);
    expect(result.needsYou[0]).toMatchObject({ kind: "question", pendingAction: "permission" });
  });

  it("classifies a task with a matching stall row as stall when last_activity_at is absent", () => {
    const t = task({ id: "t-1", statusSummary: undefined });
    const s = stall({ task_id: "t-1" });
    const result = classify([t], [s], [], NOW);
    expect(result.needsYou[0]).toMatchObject({ kind: "stall", stall: s });
  });

  it("classifies a task with a matching stall row as stall when last_activity_at equals detected_at", () => {
    const t = task({ id: "t-1", statusSummary: { last_activity_at: T_10 } });
    const s = stall({ task_id: "t-1", detected_at: T_10 });
    const result = classify([t], [s], [], NOW);
    expect(result.needsYou[0].kind).toBe("stall");
  });

  it("does not classify as stall when last_activity_at is after detected_at", () => {
    const t = task({ id: "t-1", statusSummary: { last_activity_at: T_11 } });
    const s = stall({ task_id: "t-1", detected_at: T_10 });
    const result = classify([t], [s], [], NOW);
    expect(result.needsYou).toHaveLength(0);
    expect(result.queue.other).toHaveLength(1);
  });

  it("classifies a task with active_error as error", () => {
    const t = task({
      id: "t-1",
      statusSummary: { active_error: { preview: "boom" }, last_activity_at: T_11 },
    });
    const result = classify([t], [], [], NOW);
    expect(result.needsYou[0]).toMatchObject({ kind: "error", activeError: { preview: "boom" } });
  });

  it("classifies a task with only task_error as error", () => {
    const t = task({
      id: "t-1",
      statusSummary: { task_error: { preview: "" }, last_activity_at: T_11 },
    });
    const result = classify([t], [], [], NOW);
    expect(result.needsYou[0]).toMatchObject({ kind: "error", taskError: { preview: "" } });
  });

  it("classifies a task with pull_request.aggregate_state ready as ready_to_merge", () => {
    const t = task({ id: "t-1", statusSummary: { pull_request: { aggregate_state: "ready" } } });
    const result = classify([t], [], [], NOW);
    expect(result.queue.ready_to_merge).toHaveLength(1);
  });

  it("classifies a task with pull_request.aggregate_state awaiting_review as in_review", () => {
    const t = task({
      id: "t-1",
      statusSummary: { pull_request: { aggregate_state: "awaiting_review" } },
    });
    const result = classify([t], [], [], NOW);
    expect(result.queue.in_review).toHaveLength(1);
  });

  it.each(["RUNNING", "STARTING"])(
    "classifies a task with primary_session.state %s as working",
    (state) => {
      const t = task({ id: "t-1", statusSummary: { primary_session: { id: "s-1", state } } });
      const result = classify([t], [], [], NOW);
      expect(result.queue.working).toHaveLength(1);
    },
  );

  it("classifies a task with state COMPLETED as done", () => {
    const t = task({ id: "t-1", state: "COMPLETED", statusSummary: {} });
    const result = classify([t], [], [], NOW);
    expect(result.queue.done).toHaveLength(1);
  });

  it("classifies an otherwise-matchless task as other", () => {
    const t = task({ id: "t-1", state: "TODO", statusSummary: {} });
    const result = classify([t], [], [], NOW);
    expect(result.queue.other).toHaveLength(1);
    expect(result.queue.other[0].sessionUnreadable).toBe(false);
  });

  it("excludes archived tasks entirely", () => {
    const t = task({
      id: "t-1",
      isArchived: true,
      statusSummary: { pending_action: "clarification" },
    });
    const s = stall({ task_id: "t-1" });
    const result = classify([t], [s], [], NOW);
    expect(result.needsYou).toHaveLength(0);
    expect(Object.values(result.queue).every((items) => items.length === 0)).toBe(true);
  });
});

describe("classify - precedence (first match wins)", () => {
  it("question outranks a matching stall", () => {
    const t = task({ id: "t-1", statusSummary: { pending_action: "clarification" } });
    const s = stall({ task_id: "t-1" });
    const result = classify([t], [s], [], NOW);
    expect(result.needsYou[0].kind).toBe("question");
  });

  it("stall outranks error", () => {
    const t = task({ id: "t-1", statusSummary: { active_error: { preview: "boom" } } });
    const s = stall({ task_id: "t-1" });
    const result = classify([t], [s], [], NOW);
    expect(result.needsYou[0].kind).toBe("stall");
  });

  it("error outranks ready_to_merge", () => {
    const t = task({
      id: "t-1",
      statusSummary: {
        active_error: { preview: "boom" },
        pull_request: { aggregate_state: "ready" },
      },
    });
    const result = classify([t], [], [], NOW);
    expect(result.needsYou[0].kind).toBe("error");
    expect(result.queue.ready_to_merge).toHaveLength(0);
  });

  it("ready_to_merge outranks in_review", () => {
    const t = task({ id: "t-1", statusSummary: { pull_request: { aggregate_state: "ready" } } });
    const result = classify([t], [], [], NOW);
    expect(result.queue.ready_to_merge).toHaveLength(1);
    expect(result.queue.in_review).toHaveLength(0);
  });

  it("in_review outranks working", () => {
    const t = task({
      id: "t-1",
      statusSummary: {
        pull_request: { aggregate_state: "awaiting_review" },
        primary_session: { id: "s-1", state: "RUNNING" },
      },
    });
    const result = classify([t], [], [], NOW);
    expect(result.queue.in_review).toHaveLength(1);
    expect(result.queue.working).toHaveLength(0);
  });

  it("working outranks done", () => {
    const t = task({
      id: "t-1",
      state: "COMPLETED",
      statusSummary: { primary_session: { id: "s-1", state: "RUNNING" } },
    });
    const result = classify([t], [], [], NOW);
    expect(result.queue.working).toHaveLength(1);
    expect(result.queue.done).toHaveLength(0);
  });
});

describe("classify - unreadable session", () => {
  it("statusSummary absent falls through question/error/PR/Working to done when state is COMPLETED", () => {
    const t = task({ id: "t-1", state: "COMPLETED", statusSummary: undefined });
    const result = classify([t], [], [], NOW);
    expect(result.queue.done).toHaveLength(1);
  });

  it("statusSummary absent falls through to other with sessionUnreadable when state is not COMPLETED", () => {
    const t = task({ id: "t-1", state: "TODO", statusSummary: undefined });
    const result = classify([t], [], [], NOW);
    expect(result.queue.other[0]).toMatchObject({ sessionUnreadable: true });
  });

  it("primary_session present with no state is unreadable and does not match question", () => {
    const t = task({
      id: "t-1",
      state: "TODO",
      statusSummary: { pending_action: "clarification", primary_session: { id: "s-1" } },
    });
    const result = classify([t], [], [], NOW);
    expect(result.needsYou).toHaveLength(0);
    expect(result.queue.other[0]).toMatchObject({ sessionUnreadable: true });
  });

  it("primary_session present with no state is unreadable and does not match error", () => {
    const t = task({
      id: "t-1",
      state: "TODO",
      statusSummary: { active_error: { preview: "boom" }, primary_session: { id: "s-1" } },
    });
    const result = classify([t], [], [], NOW);
    expect(result.needsYou).toHaveLength(0);
    expect(result.queue.other).toHaveLength(1);
  });

  it("primary_session present with no state is unreadable and does not match PR/Working", () => {
    const t = task({
      id: "t-1",
      state: "TODO",
      statusSummary: { pull_request: { aggregate_state: "ready" }, primary_session: { id: "s-1" } },
    });
    const result = classify([t], [], [], NOW);
    expect(result.queue.ready_to_merge).toHaveLength(0);
    expect(result.queue.other).toHaveLength(1);
  });

  it("an unreadable session with a matching stall still classifies as stall", () => {
    const t = task({ id: "t-1", statusSummary: { primary_session: { id: "s-1" } } });
    const s = stall({ task_id: "t-1" });
    const result = classify([t], [s], [], NOW);
    expect(result.needsYou[0].kind).toBe("stall");
  });

  it("null primary_session (no session) is readable, not unreadable", () => {
    const t = task({
      id: "t-1",
      statusSummary: { pending_action: "permission", primary_session: null },
    });
    const result = classify([t], [], [], NOW);
    expect(result.needsYou[0].kind).toBe("question");
  });
});

describe("classify - proposals", () => {
  it("produces one item per pending proposal independent of tasks", () => {
    const p = proposal({ id: "p-1" });
    const result = classify([], [], [p], NOW);
    expect(result.needsYou).toHaveLength(1);
    expect(result.needsYou[0]).toMatchObject({ kind: "proposal", proposal: p });
  });

  it("excludes non-pending proposals", () => {
    const p = proposal({ id: "p-1", status: "approved" });
    const result = classify([], [], [p], NOW);
    expect(result.needsYou).toHaveLength(0);
  });

  it("shows both a source task's own item and its proposal item when both qualify", () => {
    const t = task({ id: "t-1", statusSummary: { pending_action: "clarification" } });
    const p = proposal({ id: "p-1", task_id: "t-1" });
    const result = classify([t], [], [p], NOW);
    expect(result.needsYou).toHaveLength(2);
    expect(result.needsYou.map((i) => i.kind).sort()).toEqual(["proposal", "question"]);
  });
});

describe("classify - Needs you sort order (AC-002.4)", () => {
  it("sorts by reference time ascending", () => {
    const early = task({
      id: "t-early",
      statusSummary: { active_error: { preview: "x" }, last_activity_at: T_09 },
    });
    const late = task({
      id: "t-late",
      statusSummary: { active_error: { preview: "x" }, last_activity_at: T_11 },
    });
    const result = classify([late, early], [], [], NOW);
    expect(result.needsYou.map((i) => i.id)).toEqual(["t-early", "t-late"]);
  });

  it("breaks a reference-time tie by kind rank: proposal, question, stall, error", () => {
    const errorTask = task({
      id: "t-error",
      statusSummary: { active_error: { preview: "x" }, last_activity_at: T_09 },
    });
    const questionTask = task({
      id: "t-question",
      statusSummary: { pending_action: "clarification", last_activity_at: T_09 },
    });
    const stallTask = task({ id: "t-stall", statusSummary: { last_activity_at: T_09 } });
    const s = stall({ task_id: "t-stall", last_event_at: T_09, detected_at: T_09 });
    const p = proposal({ id: "p-1", created_at: T_09 });
    const result = classify([errorTask, questionTask, stallTask], [s], [p], NOW);
    expect(result.needsYou.map((i) => i.kind)).toEqual(["proposal", "question", "stall", "error"]);
  });

  it("breaks a full tie by id, code-unit ascending", () => {
    const b = task({
      id: "b",
      statusSummary: { active_error: { preview: "x" }, last_activity_at: T_09 },
    });
    const a = task({
      id: "a",
      statusSummary: { active_error: { preview: "x" }, last_activity_at: T_09 },
    });
    const result = classify([b, a], [], [], NOW);
    expect(result.needsYou.map((i) => i.id)).toEqual(["a", "b"]);
  });

  it("sorts an item with no reference time last", () => {
    const withTimeId = "t-with-time";
    const withoutTimeId = "t-without-time";
    const withTime = task({
      id: withTimeId,
      statusSummary: { active_error: { preview: "x" }, last_activity_at: T_09 },
      updatedAt: undefined,
    });
    const withoutTime = task({
      id: withoutTimeId,
      statusSummary: { active_error: { preview: "x" } },
      updatedAt: undefined,
    });
    const result = classify([withoutTime, withTime], [], [], NOW);
    expect(result.needsYou.map((i) => i.id)).toEqual([withTimeId, withoutTimeId]);
  });
});

describe("classify - Queue group sort order (AC-004.2)", () => {
  it("sorts by last_activity_at descending", () => {
    const early = task({ id: "t-early", statusSummary: { last_activity_at: T_09 } });
    const late = task({ id: "t-late", statusSummary: { last_activity_at: T_11 } });
    const result = classify([early, late], [], [], NOW);
    expect(result.queue.other.map((i) => i.id)).toEqual(["t-late", "t-early"]);
  });

  it("sorts an item with absent last_activity_at last", () => {
    const withTime = task({ id: "t-with-time", statusSummary: { last_activity_at: T_09 } });
    const withoutTime = task({ id: "t-without-time", statusSummary: {} });
    const result = classify([withoutTime, withTime], [], [], NOW);
    expect(result.queue.other.map((i) => i.id)).toEqual(["t-with-time", "t-without-time"]);
  });

  it("breaks a full tie by task id, code-unit ascending", () => {
    const b = task({ id: "b", statusSummary: { last_activity_at: T_09 } });
    const a = task({ id: "a", statusSummary: { last_activity_at: T_09 } });
    const result = classify([b, a], [], [], NOW);
    expect(result.queue.other.map((i) => i.id)).toEqual(["a", "b"]);
  });
});

describe("classify - age computation", () => {
  it("computes ageMs relative to the supplied now", () => {
    const t = task({
      id: "t-1",
      statusSummary: { active_error: { preview: "x" }, last_activity_at: T_11 },
    });
    const result = classify([t], [], [], NOW);
    expect(result.needsYou[0].ageMs).toBe(NOW_MS - new Date(T_11).getTime());
  });

  it("accepts now as an epoch-ms number as well as a Date", () => {
    const t = task({
      id: "t-1",
      statusSummary: { active_error: { preview: "x" }, last_activity_at: T_11 },
    });
    const result = classify([t], [], [], NOW_MS);
    expect(result.needsYou[0].ageMs).toBe(NOW_MS - new Date(T_11).getTime());
  });
});
