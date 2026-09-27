import { describe, expect, it, vi } from "vitest";
import type { TFunction } from "i18next";
import type {
  AttentionProposal,
  AttentionTask,
  NeedsYouErrorItem,
  NeedsYouProposalItem,
  NeedsYouQuestionItem,
  NeedsYouStallItem,
} from "@/lib/coordinator/attention";
import { resolveProposalSourceTask, severityFor, whyClearsText } from "@/lib/coordinator/item-text";

const t = vi.fn((key: string, options?: Record<string, unknown>) => {
  if (!options) return key;
  const interpolated = Object.entries(options)
    .map(([k, v]) => `${k}=${String(v)}`)
    .join(",");
  return `${key}(${interpolated})`;
}) as unknown as TFunction;

function proposal(overrides: Partial<AttentionProposal["spec"]> = {}): NeedsYouProposalItem {
  return {
    kind: "proposal",
    id: "p-1",
    referenceTimeMs: 0,
    ageMs: 0,
    proposal: {
      id: "p-1",
      status: "pending",
      task_id: null,
      created_at: "2026-09-27T00:00:00Z",
      spec: {
        title: "Add tests",
        description: "desc",
        rationale: "Coverage is thin here",
        workflow_id: "wf-1",
        step_id: "step-1",
        repository_id: "repo-1",
        source_task_id: "t-1",
        ...overrides,
      },
    },
  };
}

function task(id: string): AttentionTask {
  return { id, title: `Task ${id}` };
}

describe("severityFor", () => {
  it("is review for a proposal and decide-now for every other kind", () => {
    expect(severityFor(proposal())).toBe("review");
    const question: NeedsYouQuestionItem = {
      kind: "question",
      id: "t-1",
      task: task("t-1"),
      pendingAction: "clarification",
      referenceTimeMs: 0,
      ageMs: 0,
    };
    expect(severityFor(question)).toBe("decide-now");
  });
});

describe("resolveProposalSourceTask", () => {
  it("returns the open task matching the proposal's source_task_id", () => {
    const openTasksById = new Map([["t-1", task("t-1")]]);
    expect(resolveProposalSourceTask(proposal(), openTasksById)).toEqual(task("t-1"));
  });

  it("returns undefined when the source task is absent or archived", () => {
    expect(resolveProposalSourceTask(proposal(), new Map())).toBeUndefined();
  });

  it("returns undefined for a non-proposal item", () => {
    const stall: NeedsYouStallItem = {
      kind: "stall",
      id: "t-1",
      task: task("t-1"),
      stall: { task_id: "t-1", stalled_for_ms: 1000, last_event_at: "", detected_at: "" },
      referenceTimeMs: 0,
      ageMs: 0,
    };
    expect(resolveProposalSourceTask(stall, new Map([["t-1", task("t-1")]]))).toBeUndefined();
  });
});

describe("whyClearsText", () => {
  it("uses the proposal's rationale and the approve/edit/reject clears text", () => {
    const result = whyClearsText(proposal(), t);
    expect(result.why).toBe("Coverage is thin here");
    expect(result.clears).toBe("coordinator:approveEditOrReject");
  });

  it("uses the fixed question texts", () => {
    const question: NeedsYouQuestionItem = {
      kind: "question",
      id: "t-1",
      task: task("t-1"),
      pendingAction: "permission",
      referenceTimeMs: 0,
      ageMs: 0,
    };
    const result = whyClearsText(question, t);
    expect(result.why).toBe("coordinator:whyQuestion");
    expect(result.clears).toBe("coordinator:clearsQuestion");
  });

  it("interpolates the stalled-for duration", () => {
    const stall: NeedsYouStallItem = {
      kind: "stall",
      id: "t-1",
      task: task("t-1"),
      stall: {
        task_id: "t-1",
        stalled_for_ms: 4 * 60 * 60 * 1000 + 12 * 60 * 1000,
        last_event_at: "",
        detected_at: "",
      },
      referenceTimeMs: 0,
      ageMs: 0,
    };
    const result = whyClearsText(stall, t);
    expect(result.why).toBe("coordinator:whyStall(duration=4h 12m)");
    expect(result.clears).toBe("coordinator:clearsStall");
  });

  it("shows the active error's truncated preview when present", () => {
    const error: NeedsYouErrorItem = {
      kind: "error",
      id: "t-1",
      task: task("t-1"),
      activeError: { preview: "boom" },
      taskError: null,
      referenceTimeMs: 0,
      ageMs: 0,
    };
    const result = whyClearsText(error, t);
    expect(result.why).toBe("coordinator:whyErrorWithPreview(preview=boom)");
    expect(result.clears).toBe("coordinator:clearsError");
  });

  it("falls back to 'the task failed' with no preview, even when the task error carries one", () => {
    const error: NeedsYouErrorItem = {
      kind: "error",
      id: "t-1",
      task: task("t-1"),
      activeError: null,
      taskError: { preview: "task error preview" },
      referenceTimeMs: 0,
      ageMs: 0,
    };
    const result = whyClearsText(error, t);
    expect(result.why).toBe("coordinator:whyErrorTaskFailed");
  });
});
