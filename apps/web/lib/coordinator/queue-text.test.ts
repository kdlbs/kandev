import { describe, expect, it } from "vitest";
import type { TFunction } from "i18next";
import type { AttentionTask, QueueItem } from "@/lib/coordinator/attention";
import { agentStateLabel, otherRowStatusText } from "./queue-text";

const t = ((key: string) => key) as TFunction;

function task(overrides: Partial<AttentionTask> = {}): AttentionTask {
  return { id: "t-1", title: "Task 1", ...overrides };
}

function queueItem(overrides: Partial<QueueItem> = {}): QueueItem {
  return {
    group: "other",
    id: "t-1",
    task: task(),
    lastActivityAtMs: undefined,
    ageMs: undefined,
    ...overrides,
  };
}

describe("agentStateLabel", () => {
  it("labels RUNNING", () => {
    expect(agentStateLabel(t, "RUNNING")).toBe("coordinator:agentStateRunning");
  });

  it("labels anything else as Starting", () => {
    expect(agentStateLabel(t, "STARTING")).toBe("coordinator:agentStateStarting");
  });
});

describe("otherRowStatusText", () => {
  it("is undefined outside the Other group", () => {
    const item = queueItem({ group: "working", sessionUnreadable: true });
    expect(otherRowStatusText(t, item, task())).toBeUndefined();
  });

  it("shows the underivable text when the session is unreadable", () => {
    const item = queueItem({ sessionUnreadable: true });
    expect(otherRowStatusText(t, item, task())).toBe("coordinator:positionUnderivable");
  });

  it("shows 'no session' when the task has no primary session", () => {
    const item = queueItem({ sessionUnreadable: false });
    expect(otherRowStatusText(t, item, task({ statusSummary: { primary_session: null } }))).toBe(
      "coordinator:noSession",
    );
  });

  it("is undefined otherwise", () => {
    const item = queueItem({ sessionUnreadable: false });
    expect(
      otherRowStatusText(
        t,
        item,
        task({ statusSummary: { primary_session: { id: "s-1", state: "FAILED" } } }),
      ),
    ).toBeUndefined();
  });
});
