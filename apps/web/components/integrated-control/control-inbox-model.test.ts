import { describe, expect, it } from "vitest";
import type { ClarificationInboxBundle, InboxRecoveryBlock } from "@/lib/types/clarification-inbox";
import {
  countInbox,
  groupInboxSection,
  workspaceInboxFigures,
  type ControlInboxSection,
  type ControlInboxWorkspaceRead,
} from "./control-inbox-model";

const AT_ONE = "2026-10-20T01:00:00Z";

const bundle = (id: string, createdAt: string): ClarificationInboxBundle => ({
  pending_id: id,
  task_id: `task-${id}`,
  session_id: `session-${id}`,
  session_state: "WAITING_FOR_INPUT",
  task_title: `Task ${id}`,
  created_at: createdAt,
  context: "",
  messages: [],
});

const block = (id: string, createdAt: string): InboxRecoveryBlock => ({
  block_id: id,
  task_id: `task-${id}`,
  task_title: `Task ${id}`,
  session_id: `session-${id}`,
  session_state: "WAITING_FOR_INPUT",
  reason: "unknown_prompt_outcome",
  created_at: createdAt,
  updated_at: createdAt,
});

const ready = <T>(items: T[], truncated = false): ControlInboxSection<T> => ({
  status: "ready",
  items,
  truncated,
});

const read = (
  workspaceId: string,
  over: Partial<Omit<ControlInboxWorkspaceRead, "workspaceId">> = {},
): ControlInboxWorkspaceRead => ({
  workspaceId,
  answerable: ready([]),
  earlier: ready([]),
  recovery: ready([]),
  failed: ready([]),
  ...over,
});

describe("control inbox model", () => {
  it("groups a section by workspace, oldest first, and omits empty ready workspaces", () => {
    const reads = [
      read("w1", {
        answerable: ready([bundle("b", "2026-10-20T02:00:00Z"), bundle("a", AT_ONE)]),
      }),
      read("w2"),
      read("w3", { answerable: { status: "denied", items: [], truncated: false } }),
    ];
    const groups = groupInboxSection(reads, (item) => item.answerable);
    expect(groups.map((group) => group.workspaceId)).toEqual(["w1", "w3"]);
    expect(groups[0].items.map((item) => item.pending_id)).toEqual(["a", "b"]);
    expect(groups[1].status).toBe("denied");
  });

  it("counts pending as questions plus earlier-turn questions plus recovery decisions", () => {
    const counts = countInbox([
      read("w1", {
        answerable: ready([bundle("a", AT_ONE)]),
        earlier: ready([bundle("b", AT_ONE), bundle("c", AT_ONE)]),
      }),
      read("w2", { recovery: ready([block("r", AT_ONE)]) }),
    ]);
    expect(counts).toEqual({
      pending: 4,
      answerable: 1,
      earlier: 2,
      recovery: 1,
      failed: 0,
      partial: false,
    });
  });

  it("marks the counts partial when a section failed or was truncated", () => {
    expect(
      countInbox([read("w1", { failed: { status: "error", items: [], truncated: false } })])
        .partial,
    ).toBe(true);
    expect(countInbox([read("w1", { earlier: ready([], true) })]).partial).toBe(true);
  });

  it("reports per-workspace figures with the oldest waiting instant", () => {
    const figures = workspaceInboxFigures([
      read("w1", {
        answerable: ready([bundle("a", "2026-10-20T03:00:00Z")]),
        recovery: ready([block("r", AT_ONE)]),
      }),
      read("w2"),
    ]);
    expect(figures).toEqual({
      w1: { answerable: 1, earlier: 0, recovery: 1, oldest: AT_ONE },
    });
  });
});
