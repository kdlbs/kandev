import { describe, expect, it } from "vitest";
import { sessionId as toSessionId, taskId as toTaskId, type Message } from "@/lib/types/http";
import {
  dropSupersededEmptyTurnNotices,
  filterVisibleMessages,
  hasFailedAgentBootAfter,
  hasSessionRecoveryResolutionAfter,
  hasSuccessfulAgentBootAfter,
  isSuccessfulScriptExecutionMetadata,
} from "./processed-message-filtering";

const ERROR_AT = "2026-05-30T00:00:00Z";
const AFTER = "2026-05-30T00:01:00Z";
const BEFORE = "2026-05-29T23:59:00Z";

function bootMessage(createdAt: string, metadata: Record<string, unknown> = {}): Message {
  return {
    id: `boot-${createdAt}-${JSON.stringify(metadata)}`,
    session_id: toSessionId("s1"),
    task_id: toTaskId("t1"),
    author_type: "agent",
    content: "",
    type: "script_execution",
    created_at: createdAt,
    metadata: {
      script_type: "agent_boot",
      agent_name: "Mock",
      is_resuming: true,
      status: "exited",
      ...metadata,
    },
  } as Message;
}

describe("hasSuccessfulAgentBootAfter", () => {
  it("returns true when the agent was resumed after the failure", () => {
    expect(hasSuccessfulAgentBootAfter([bootMessage(AFTER)], ERROR_AT)).toBe(true);
  });

  it("returns true when a fresh start re-established the agent after the failure", () => {
    expect(
      hasSuccessfulAgentBootAfter([bootMessage(AFTER, { is_resuming: false })], ERROR_AT),
    ).toBe(true);
  });

  it("returns true when an explicit zero exit code reports success", () => {
    expect(hasSuccessfulAgentBootAfter([bootMessage(AFTER, { exit_code: 0 })], ERROR_AT)).toBe(
      true,
    );
  });

  it("returns false when the only successful boot predates the failure", () => {
    expect(hasSuccessfulAgentBootAfter([bootMessage(BEFORE)], ERROR_AT)).toBe(false);
  });

  it("returns false when the resume attempt failed", () => {
    expect(hasSuccessfulAgentBootAfter([bootMessage(AFTER, { status: "failed" })], ERROR_AT)).toBe(
      false,
    );
  });

  it("returns false when the boot exited non-zero", () => {
    expect(hasSuccessfulAgentBootAfter([bootMessage(AFTER, { exit_code: 1 })], ERROR_AT)).toBe(
      false,
    );
  });

  it("returns false while the resume is still running", () => {
    expect(hasSuccessfulAgentBootAfter([bootMessage(AFTER, { status: "running" })], ERROR_AT)).toBe(
      false,
    );
  });

  it("ignores non-boot scripts that finished after the failure", () => {
    const setup = bootMessage(AFTER, { script_type: "setup" });
    expect(hasSuccessfulAgentBootAfter([setup], ERROR_AT)).toBe(false);
  });

  it("ignores messages that are not script executions", () => {
    const chat = { ...bootMessage(AFTER), type: "message" } as Message;
    expect(hasSuccessfulAgentBootAfter([chat], ERROR_AT)).toBe(false);
  });

  it("returns false for unparseable or missing timestamps", () => {
    expect(hasSuccessfulAgentBootAfter([bootMessage("")], ERROR_AT)).toBe(false);
    expect(hasSuccessfulAgentBootAfter([bootMessage(AFTER)], "")).toBe(false);
    expect(hasSuccessfulAgentBootAfter(undefined, ERROR_AT)).toBe(false);
    expect(hasSuccessfulAgentBootAfter([], ERROR_AT)).toBe(false);
  });

  it("finds the boot even when it is not the newest message", () => {
    const later = { ...bootMessage("2026-05-30T00:02:00Z"), type: "message" } as Message;
    expect(hasSuccessfulAgentBootAfter([bootMessage(AFTER), later], ERROR_AT)).toBe(true);
  });
});

describe("recovery resolution metadata", () => {
  it("resolves a card only when the session timestamp is newer", () => {
    expect(hasSessionRecoveryResolutionAfter({ recovery_resolved_at: AFTER }, ERROR_AT)).toBe(true);
    expect(hasSessionRecoveryResolutionAfter({ recovery_resolved_at: BEFORE }, ERROR_AT)).toBe(
      false,
    );
  });

  it("rejects missing or invalid recovery timestamps", () => {
    expect(hasSessionRecoveryResolutionAfter(undefined, ERROR_AT)).toBe(false);
    expect(hasSessionRecoveryResolutionAfter({ recovery_resolved_at: "invalid" }, ERROR_AT)).toBe(
      false,
    );
    expect(hasSessionRecoveryResolutionAfter({ recovery_resolved_at: AFTER }, "invalid")).toBe(
      false,
    );
  });
});

describe("isSuccessfulScriptExecutionMetadata", () => {
  it("uses the shared exited-zero success rule", () => {
    expect(isSuccessfulScriptExecutionMetadata({ status: "exited" })).toBe(true);
    expect(isSuccessfulScriptExecutionMetadata({ status: "exited", exit_code: 0 })).toBe(true);
    expect(isSuccessfulScriptExecutionMetadata({ status: "exited", exit_code: 1 })).toBe(false);
    expect(isSuccessfulScriptExecutionMetadata({ status: "running" })).toBe(false);
  });

  it("recognizes the terminal success status used by workflow scripts", () => {
    expect(isSuccessfulScriptExecutionMetadata({ status: "succeeded", exit_code: 0 })).toBe(true);
    expect(isSuccessfulScriptExecutionMetadata({ status: "succeeded", exit_code: 1 })).toBe(false);
  });
});

function baseMessage(overrides: Partial<Message>): Message {
  return {
    id: "m",
    session_id: toSessionId("s1"),
    task_id: toTaskId("t1"),
    author_type: "agent",
    content: "",
    type: "message",
    created_at: "2026-05-30T00:00:00Z",
    ...overrides,
  } as Message;
}

const PLAN_TURN_ID = "turn-1";
const AGENT_PLAN_TYPE = "agent_plan";
const FIRST_PLAN_CONTENT = "# Plan\n\n1. Read";
const LATEST_PLAN_CONTENT = `${FIRST_PLAN_CONTENT}\n2. Write`;
const LEGACY_PLAN_1_ID = "legacy-plan-1";
const LEGACY_PLAN_2_ID = "legacy-plan-2";

function planMessage(id: string, content: string, overrides: Partial<Message> = {}): Message {
  return baseMessage({
    id,
    turn_id: PLAN_TURN_ID,
    type: AGENT_PLAN_TYPE,
    content,
    ...overrides,
  });
}

function emptyTurnNotice(turnId: string): Message {
  return baseMessage({
    id: `empty-turn-${turnId}`,
    turn_id: turnId,
    type: "status",
    content: "The agent finished without producing any output.",
    metadata: { variant: "warning", empty_turn: true },
  });
}

describe("dropSupersededEmptyTurnNotices", () => {
  it("drops the notice once real agent text arrives on the same turn", () => {
    const messages = [
      baseMessage({ id: "u1", turn_id: "turn-1", author_type: "user", content: "hi" }),
      emptyTurnNotice("turn-1"),
      baseMessage({ id: "a1", turn_id: "turn-1", content: "here you go" }),
    ];
    const result = dropSupersededEmptyTurnNotices(messages);
    expect(result.map((m) => m.id)).toEqual(["u1", "a1"]);
  });

  it("keeps the notice when the turn never received output", () => {
    const messages = [
      baseMessage({ id: "u1", turn_id: "turn-1", author_type: "user", content: "hi" }),
      emptyTurnNotice("turn-1"),
    ];
    expect(dropSupersededEmptyTurnNotices(messages)).toHaveLength(2);
  });

  it("keeps the notice when the output belongs to a different turn", () => {
    const messages = [
      emptyTurnNotice("turn-1"),
      baseMessage({ id: "a1", turn_id: "turn-2", content: "unrelated" }),
    ];
    expect(dropSupersededEmptyTurnNotices(messages)).toHaveLength(2);
  });

  it("keeps the notice when only status/thinking rows follow on the same turn", () => {
    const messages = [
      emptyTurnNotice("turn-1"),
      baseMessage({ id: "s1", turn_id: "turn-1", type: "status", content: "New session started" }),
      baseMessage({ id: "th1", turn_id: "turn-1", type: "thinking", content: "pondering" }),
    ];
    expect(dropSupersededEmptyTurnNotices(messages)).toHaveLength(3);
  });

  it("drops the notice when a tool call lands on the same turn", () => {
    const messages = [
      emptyTurnNotice("turn-1"),
      baseMessage({
        id: "tc1",
        turn_id: "turn-1",
        type: "tool_call",
        metadata: { tool_call_id: "call-1" },
      }),
    ];
    const result = dropSupersededEmptyTurnNotices(messages);
    expect(result.map((m) => m.id)).toEqual(["tc1"]);
  });

  it("drops the notice when a search tool lands on the same turn", () => {
    const messages = [
      emptyTurnNotice("turn-1"),
      baseMessage({ id: "search-1", turn_id: "turn-1", type: "tool_search" }),
    ];
    const result = dropSupersededEmptyTurnNotices(messages);
    expect(result.map((m) => m.id)).toEqual(["search-1"]);
  });
});

describe("filterVisibleMessages empty-turn notice supersession", () => {
  it("drops an empty-turn notice once an approved permission_request lands on its turn, even though approval hides that request from the visible list", () => {
    const notice = emptyTurnNotice("turn-1");
    const approvedPermission = baseMessage({
      id: "perm-1",
      turn_id: "turn-1",
      type: "permission_request",
      metadata: { status: "approved" },
    });

    expect(
      filterVisibleMessages([notice, approvedPermission], new Set<string>(), new Set<string>()).map(
        (message) => message.id,
      ),
    ).toEqual([]);
  });

  it("drops an empty-turn notice once a permission_request tied to a visible tool call lands on its turn", () => {
    const notice = emptyTurnNotice("turn-2");
    const linkedPermission = baseMessage({
      id: "perm-2",
      turn_id: "turn-2",
      type: "permission_request",
      metadata: { tool_call_id: "call-1" },
    });

    expect(
      filterVisibleMessages(
        [notice, linkedPermission],
        new Set<string>(["call-1"]),
        new Set<string>(),
      ).map((message) => message.id),
    ).toEqual([]);
  });
});

const filterPlanMessages = (messages: Message[]) =>
  filterVisibleMessages(messages, new Set<string>(), new Set<string>());

describe("filterVisibleMessages correlated agent plans", () => {
  // @covers AC-AGENTS-AGENT-PLAN-STREAM-COALESCING-001.1
  it("keeps only the latest snapshot for a correlated plan stream", () => {
    const correlation = { tool_call_id: "agent-plan:correlation-1" };
    const first = planMessage("plan-1", FIRST_PLAN_CONTENT, { metadata: correlation });
    const latest = planMessage("plan-2", LATEST_PLAN_CONTENT, { metadata: correlation });

    expect(filterPlanMessages([first, latest]).map((message) => message.id)).toEqual(["plan-2"]);
  });

  it("keeps only the final delivery when repeated snapshots share a message id", () => {
    const correlation = { tool_call_id: "agent-plan:correlation-1" };
    const first = planMessage("plan-1", FIRST_PLAN_CONTENT, { metadata: correlation });
    const latest = planMessage("plan-1", LATEST_PLAN_CONTENT, { metadata: correlation });

    expect(filterPlanMessages([first, latest]).map((message) => message.content)).toEqual([
      LATEST_PLAN_CONTENT,
    ]);
  });

  // @covers AC-AGENTS-AGENT-PLAN-STREAM-COALESCING-001.2
  it("keeps separate correlated plan streams in one turn", () => {
    const first = planMessage("plan-1", "# First plan", {
      metadata: { tool_call_id: "agent-plan:correlation-1" },
    });
    const second = planMessage("plan-2", "# Second plan", {
      metadata: { tool_call_id: "agent-plan:correlation-2" },
    });

    expect(filterPlanMessages([first, second]).map((message) => message.id)).toEqual([
      "plan-1",
      "plan-2",
    ]);
  });
});

describe("filterVisibleMessages legacy agent plans", () => {
  // @covers AC-AGENTS-AGENT-PLAN-STREAM-COALESCING-001.4
  it("collapses a contiguous same-turn legacy prefix chain", () => {
    const first = planMessage(LEGACY_PLAN_1_ID, FIRST_PLAN_CONTENT);
    const latest = planMessage(LEGACY_PLAN_2_ID, LATEST_PLAN_CONTENT);

    expect(filterPlanMessages([first, latest]).map((message) => message.id)).toEqual([
      LEGACY_PLAN_2_ID,
    ]);
  });

  it("keeps legacy plans separated by another conversation item", () => {
    const first = planMessage(LEGACY_PLAN_1_ID, FIRST_PLAN_CONTENT);
    const message = baseMessage({ id: "message-1", turn_id: PLAN_TURN_ID, content: "Working" });
    const latest = planMessage(LEGACY_PLAN_2_ID, LATEST_PLAN_CONTENT);

    expect(filterPlanMessages([first, message, latest]).map((item) => item.id)).toEqual([
      LEGACY_PLAN_1_ID,
      "message-1",
      LEGACY_PLAN_2_ID,
    ]);
  });

  it("keeps legacy prefix plans from different turns", () => {
    const first = planMessage(LEGACY_PLAN_1_ID, "# Plan");
    const second = planMessage(LEGACY_PLAN_2_ID, FIRST_PLAN_CONTENT, { turn_id: "turn-2" });

    expect(filterPlanMessages([first, second]).map((message) => message.id)).toEqual([
      LEGACY_PLAN_1_ID,
      LEGACY_PLAN_2_ID,
    ]);
  });

  it("keeps same-turn legacy plans when the content is not a prefix", () => {
    const first = planMessage(LEGACY_PLAN_1_ID, "# First plan");
    const second = planMessage(LEGACY_PLAN_2_ID, "# Different plan");

    expect(filterPlanMessages([first, second]).map((message) => message.id)).toEqual([
      LEGACY_PLAN_1_ID,
      LEGACY_PLAN_2_ID,
    ]);
  });
});

describe("filterVisibleMessages recovery history", () => {
  const RECOVERY_MESSAGE = "Could not resume the saved session.";
  const FIRST_RECOVERY_ID = "recovery-1";

  it("keeps a session failure after later agent output arrives", () => {
    const failure = baseMessage({
      id: FIRST_RECOVERY_ID,
      type: "status",
      content: RECOVERY_MESSAGE,
      metadata: { recovery_actions: true, recovery_stamp: "failure-1" },
    });
    const output = baseMessage({
      id: "agent-1",
      type: "message",
      content: "The resumed session is ready.",
      created_at: AFTER,
    });

    expect(
      filterVisibleMessages([failure, output], new Set(), new Set()).map((message) => message.id),
    ).toEqual([FIRST_RECOVERY_ID, "agent-1"]);
  });

  it("collapses duplicate deliveries of one failure while keeping later failures", () => {
    const first = baseMessage({
      id: FIRST_RECOVERY_ID,
      type: "status",
      content: RECOVERY_MESSAGE,
      metadata: { recovery_actions: true, recovery_stamp: "failure-1" },
    });
    const duplicate = { ...first, id: `${FIRST_RECOVERY_ID}-duplicate` };
    const second = baseMessage({
      id: "recovery-2",
      type: "status",
      content: RECOVERY_MESSAGE,
      metadata: { recovery_actions: true, recovery_stamp: "failure-2" },
      created_at: AFTER,
    });

    expect(
      filterVisibleMessages([first, duplicate, second], new Set(), new Set()).map(
        (message) => message.id,
      ),
    ).toEqual([FIRST_RECOVERY_ID, "recovery-2"]);
  });
});

describe("hasFailedAgentBootAfter", () => {
  it("returns true when a boot after the failure reports status failed", () => {
    expect(hasFailedAgentBootAfter([bootMessage(AFTER, { status: "failed" })], ERROR_AT)).toBe(
      true,
    );
  });

  it("returns true when a boot after the failure exits non-zero", () => {
    expect(hasFailedAgentBootAfter([bootMessage(AFTER, { exit_code: 1 })], ERROR_AT)).toBe(true);
  });

  it("returns false for a successful boot", () => {
    expect(hasFailedAgentBootAfter([bootMessage(AFTER)], ERROR_AT)).toBe(false);
  });

  it("returns false while the boot is still running", () => {
    expect(hasFailedAgentBootAfter([bootMessage(AFTER, { status: "running" })], ERROR_AT)).toBe(
      false,
    );
  });

  it("returns false when the failed boot predates the failure", () => {
    expect(hasFailedAgentBootAfter([bootMessage(BEFORE, { status: "failed" })], ERROR_AT)).toBe(
      false,
    );
  });

  it("ignores non-boot scripts that failed after the failure", () => {
    const setup = bootMessage(AFTER, { script_type: "setup", status: "failed" });
    expect(hasFailedAgentBootAfter([setup], ERROR_AT)).toBe(false);
  });
});
