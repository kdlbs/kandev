import { describe, expect, it } from "vitest";
import { deriveCopilotItemId, normalizeCopilotItemId } from "./copilot-id";
import type {
  AttentionProposal,
  AttentionTask,
  NeedsYouProposalItem,
  NeedsYouStallItem,
  QueueItem,
} from "./attention";

function task(overrides: Partial<AttentionTask> = {}): AttentionTask {
  return { id: "task-1", title: "Fix the thing", ...overrides };
}

const PROPOSAL_ID = "proposal-1";

function proposal(overrides: Partial<AttentionProposal> = {}): AttentionProposal {
  return {
    id: PROPOSAL_ID,
    status: "pending",
    task_id: null,
    created_at: "2026-09-28T00:00:00Z",
    spec: {
      title: "Split into two tasks",
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

describe("normalizeCopilotItemId", () => {
  it("collapses whitespace runs, including line breaks, to one space", () => {
    expect(normalizeCopilotItemId("KAN\n  418\tfix")).toBe("KAN 418 fix");
  });

  it("trims leading and trailing whitespace", () => {
    expect(normalizeCopilotItemId("  KAN-418  ")).toBe("KAN-418");
  });

  it("replaces each first colon-space run with a hyphen until none remain", () => {
    expect(normalizeCopilotItemId("Epic: Sub: Task")).toBe("Epic - Sub - Task");
  });

  it("does not alter a colon with no following space", () => {
    expect(normalizeCopilotItemId("KAN:418")).toBe("KAN:418");
  });
});

describe("deriveCopilotItemId", () => {
  it("uses the task identifier when present", () => {
    const item: NeedsYouStallItem = {
      id: "stall-1",
      kind: "stall",
      referenceTimeMs: undefined,
      ageMs: undefined,
      task: task({ identifier: "KAN-418" }),
      stall: { task_id: "task-1", stalled_for_ms: 0, last_event_at: "", detected_at: "" },
    };
    expect(deriveCopilotItemId(item, new Map())).toBe("KAN-418");
  });

  it("falls back to the task title when identifier is absent", () => {
    const item: NeedsYouStallItem = {
      id: "stall-1",
      kind: "stall",
      referenceTimeMs: undefined,
      ageMs: undefined,
      task: task({ identifier: undefined, title: "Fix the thing" }),
      stall: { task_id: "task-1", stalled_for_ms: 0, last_event_at: "", detected_at: "" },
    };
    expect(deriveCopilotItemId(item, new Map())).toBe("Fix the thing");
  });

  it("uses the source task's identifier for a proposal with a source task", () => {
    const sourceTask = task({ id: "task-9", identifier: "KAN-9" });
    const openTasksById = new Map([[sourceTask.id, sourceTask]]);
    const item: NeedsYouProposalItem = {
      id: PROPOSAL_ID,
      kind: "proposal",
      referenceTimeMs: undefined,
      ageMs: undefined,
      proposal: proposal({ spec: { ...proposal().spec, source_task_id: "task-9" } }),
    };
    expect(deriveCopilotItemId(item, openTasksById)).toBe("KAN-9");
  });

  it("uses the source task's title when it has no identifier", () => {
    const sourceTask = task({ id: "task-9", identifier: undefined, title: "Source task" });
    const openTasksById = new Map([[sourceTask.id, sourceTask]]);
    const item: NeedsYouProposalItem = {
      id: PROPOSAL_ID,
      kind: "proposal",
      referenceTimeMs: undefined,
      ageMs: undefined,
      proposal: proposal({ spec: { ...proposal().spec, source_task_id: "task-9" } }),
    };
    expect(deriveCopilotItemId(item, openTasksById)).toBe("Source task");
  });

  it("uses the proposal's own title when it has no source task, not a fallback label", () => {
    const item: NeedsYouProposalItem = {
      id: PROPOSAL_ID,
      kind: "proposal",
      referenceTimeMs: undefined,
      ageMs: undefined,
      proposal: proposal({
        spec: { ...proposal().spec, source_task_id: "", title: "New feature" },
      }),
    };
    expect(deriveCopilotItemId(item, new Map())).toBe("New feature");
  });

  it("normalizes a title with a line break, repeated spaces and a colon-space run", () => {
    const item: NeedsYouProposalItem = {
      id: PROPOSAL_ID,
      kind: "proposal",
      referenceTimeMs: undefined,
      ageMs: undefined,
      proposal: proposal({
        spec: { ...proposal().spec, source_task_id: "", title: "Epic:  Split\ninto   two" },
      }),
    };
    expect(deriveCopilotItemId(item, new Map())).toBe("Epic - Split into two");
  });

  it("derives from a queue item's task", () => {
    const item: QueueItem = {
      group: "working",
      id: "task-1",
      task: task({ identifier: "KAN-418" }),
      lastActivityAtMs: undefined,
      ageMs: undefined,
    };
    expect(deriveCopilotItemId(item, new Map())).toBe("KAN-418");
  });
});
