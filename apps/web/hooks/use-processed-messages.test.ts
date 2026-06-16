import { describe, it, expect } from "vitest";
import {
  sessionId as toSessionId,
  taskId as toTaskId,
  type Message,
  type MessageType,
} from "@/lib/types/http";
import type { RichMetadata } from "@/components/task/chat/types";
import {
  buildGroupedRenderItems,
  collapseTodoSnapshotsPerTurn,
  deduplicateAgentBootResumes,
  insertLastAgentErrorItem,
  isAgentBootResumeMessage,
  isSetupScriptMessage,
  messageListMapsEqual,
  messageMapsEqualByIdentity,
  reconcileRenderItems,
  type RenderItem,
} from "./use-processed-messages";

function makeMessage(
  id: string,
  type: MessageType,
  metadata?: Record<string, unknown>,
  content = "",
): Message {
  return {
    id,
    session_id: toSessionId("s1"),
    task_id: toTaskId("t1"),
    author_type: "agent",
    content,
    type,
    metadata,
    created_at: "",
  };
}

function makeTodo(
  id: string,
  turnId: string | undefined,
  todos: Array<{ text: string; done?: boolean }>,
): Message {
  return { ...makeMessage(id, "todo", { todos }), turn_id: turnId };
}

function toolExecute(id: string, turnId = "turn-1"): Message {
  return {
    ...makeMessage(id, "tool_execute", {
      status: "complete",
      normalized: { shell_exec: { command: "gh pr checks", output: { exit_code: 0 } } },
    }),
    content: "gh pr checks",
    turn_id: turnId,
  };
}

function bootStarted(id: string): Message {
  return makeMessage(id, "script_execution", {
    script_type: "agent_boot",
    agent_name: "Mock",
    is_resuming: false,
    status: "exited",
  });
}

function bootResumed(id: string): Message {
  return makeMessage(id, "script_execution", {
    script_type: "agent_boot",
    agent_name: "Mock",
    is_resuming: true,
    status: "exited",
  });
}

describe("isAgentBootResumeMessage", () => {
  it("returns true for script_execution agent_boot with is_resuming=true", () => {
    expect(isAgentBootResumeMessage(bootResumed("r1"))).toBe(true);
  });

  it("returns false for a Started (non-resuming) agent_boot", () => {
    expect(isAgentBootResumeMessage(bootStarted("s1"))).toBe(false);
  });

  it("returns false for a setup/cleanup script", () => {
    const setup = makeMessage("x", "script_execution", {
      script_type: "setup",
      is_resuming: true,
    });
    expect(isAgentBootResumeMessage(setup)).toBe(false);
  });

  it("returns false for unrelated message types", () => {
    expect(isAgentBootResumeMessage(makeMessage("m1", "message"))).toBe(false);
  });

  it("returns false when metadata is missing", () => {
    const msg = makeMessage("x", "script_execution");
    expect(isAgentBootResumeMessage(msg)).toBe(false);
  });
});

describe("isSetupScriptMessage", () => {
  it("returns true for a script_execution with script_type=setup", () => {
    const msg = makeMessage("x", "script_execution", { script_type: "setup", status: "exited" });
    expect(isSetupScriptMessage(msg)).toBe(true);
  });

  it("returns false for agent_boot and cleanup scripts", () => {
    expect(isSetupScriptMessage(bootStarted("s1"))).toBe(false);
    const cleanup = makeMessage("c1", "script_execution", { script_type: "cleanup" });
    expect(isSetupScriptMessage(cleanup)).toBe(false);
  });

  it("returns false for non-script messages", () => {
    expect(isSetupScriptMessage(makeMessage("m1", "message"))).toBe(false);
  });

  it("returns false when metadata is missing", () => {
    expect(isSetupScriptMessage(makeMessage("x", "script_execution"))).toBe(false);
  });
});

describe("deduplicateAgentBootResumes", () => {
  it("returns an empty list unchanged", () => {
    expect(deduplicateAgentBootResumes([])).toEqual([]);
  });

  it("returns the list unchanged when there are no resume messages", () => {
    const messages = [bootStarted("s1"), makeMessage("m1", "message", undefined, "hi")];
    expect(deduplicateAgentBootResumes(messages)).toEqual(messages);
  });

  it("returns the list unchanged when there is exactly one resume message", () => {
    const messages = [
      bootStarted("s1"),
      makeMessage("m1", "message", undefined, "hi"),
      bootResumed("r1"),
    ];
    expect(deduplicateAgentBootResumes(messages)).toEqual(messages);
  });

  it("keeps only the last resume message when multiple exist", () => {
    const messages = [bootResumed("r1"), bootResumed("r2"), bootResumed("r3")];
    const result = deduplicateAgentBootResumes(messages);
    expect(result).toHaveLength(1);
    expect(result[0].id).toBe("r3");
  });

  it("preserves Started and non-boot messages while deduping resumes", () => {
    const started = bootStarted("s1");
    const userMsg = makeMessage("m1", "message", undefined, "hello");
    const r1 = bootResumed("r1");
    const r2 = bootResumed("r2");
    const agentMsg = makeMessage("m2", "message", undefined, "reply");
    const r3 = bootResumed("r3");

    const result = deduplicateAgentBootResumes([started, userMsg, r1, r2, agentMsg, r3]);

    expect(result.map((m) => m.id)).toEqual(["s1", "m1", "m2", "r3"]);
  });

  it("does not touch setup/cleanup script executions", () => {
    const setup = makeMessage("x", "script_execution", {
      script_type: "setup",
      status: "exited",
    });
    const messages = [setup, bootResumed("r1"), bootResumed("r2")];
    const result = deduplicateAgentBootResumes(messages);
    expect(result.map((m) => m.id)).toEqual(["x", "r2"]);
  });
});

describe("collapseTodoSnapshotsPerTurn", () => {
  it("returns the list unchanged when there are no todo messages", () => {
    const messages = [makeMessage("m1", "message", undefined, "hi")];
    expect(collapseTodoSnapshotsPerTurn(messages)).toEqual(messages);
  });

  it("returns a single todo message unchanged", () => {
    const todo = makeTodo("t1", "turn-1", [{ text: "step 1" }]);
    const result = collapseTodoSnapshotsPerTurn([todo]);
    expect(result).toEqual([todo]);
    expect((result[0].metadata as RichMetadata).previous_todo_snapshots).toBeUndefined();
  });

  it("keeps only the latest todo per turn and attaches prior snapshots", () => {
    const userMsg = makeMessage("u1", "message", undefined, "go");
    const t1 = makeTodo("t1", "turn-1", [{ text: "a" }]);
    const t2 = makeTodo("t2", "turn-1", [{ text: "a", done: true }, { text: "b" }]);
    const t3 = makeTodo("t3", "turn-1", [
      { text: "a", done: true },
      { text: "b", done: true },
      { text: "c" },
    ]);
    const result = collapseTodoSnapshotsPerTurn([userMsg, t1, t2, t3]);

    expect(result.map((m) => m.id)).toEqual(["u1", "t3"]);
    const meta = result[1].metadata as RichMetadata;
    expect(meta.previous_todo_snapshots).toHaveLength(2);
    expect(meta.previous_todo_snapshots?.[0].todos).toEqual([{ text: "a" }]);
    expect(meta.previous_todo_snapshots?.[1].todos).toEqual([
      { text: "a", done: true },
      { text: "b" },
    ]);
    // Latest todos remain reachable on the kept message.
    expect(meta.todos).toEqual([
      { text: "a", done: true },
      { text: "b", done: true },
      { text: "c" },
    ]);
  });

  it("collapses todos independently per turn", () => {
    const a1 = makeTodo("a1", "turn-A", [{ text: "x" }]);
    const a2 = makeTodo("a2", "turn-A", [{ text: "x", done: true }]);
    const b1 = makeTodo("b1", "turn-B", [{ text: "y" }]);
    const result = collapseTodoSnapshotsPerTurn([a1, a2, b1]);

    expect(result.map((m) => m.id)).toEqual(["a2", "b1"]);
    expect((result[0].metadata as RichMetadata).previous_todo_snapshots).toHaveLength(1);
    expect((result[1].metadata as RichMetadata).previous_todo_snapshots).toBeUndefined();
  });

  it("preserves todo messages without a turn_id", () => {
    const orphan = makeTodo("o1", undefined, [{ text: "orphan" }]);
    const t1 = makeTodo("t1", "turn-1", [{ text: "a" }]);
    const t2 = makeTodo("t2", "turn-1", [{ text: "a", done: true }]);
    const result = collapseTodoSnapshotsPerTurn([orphan, t1, t2]);

    expect(result.map((m) => m.id)).toEqual(["o1", "t2"]);
  });

  it("does not mutate input messages", () => {
    const t1 = makeTodo("t1", "turn-1", [{ text: "a" }]);
    const t2 = makeTodo("t2", "turn-1", [{ text: "a", done: true }]);
    collapseTodoSnapshotsPerTurn([t1, t2]);
    expect(t1.metadata).toEqual({ todos: [{ text: "a" }] });
    expect(t2.metadata).toEqual({ todos: [{ text: "a", done: true }] });
  });
});

describe("buildGroupedRenderItems prepare progress placement", () => {
  it("does not inject prepare progress into a partial tool-only history window", () => {
    const partialWindow = [toolExecute("tool-1"), toolExecute("tool-2")];

    const result = buildGroupedRenderItems(partialWindow, "s1", {
      canAnchorPrepareProgress: false,
    });

    expect(result.map((item) => item.type)).toEqual(["turn_group"]);
  });

  it("injects prepare progress when the session start is loaded", () => {
    const initialPrompt = makeMessage("user-1", "message", undefined, "start");

    const result = buildGroupedRenderItems([initialPrompt], "s1", {
      canAnchorPrepareProgress: true,
    });

    expect(result.map((item) => item.type)).toEqual(["message", "prepare_progress"]);
  });
});

describe("insertLastAgentErrorItem", () => {
  it("inserts the notice after the nearest message before the error timestamp", () => {
    const before = {
      ...makeMessage("before", "message", undefined, "before"),
      created_at: "2026-06-14T10:00:00Z",
    };
    const after = {
      ...makeMessage("after", "message", undefined, "after"),
      created_at: "2026-06-14T10:10:00Z",
    };
    const items = buildGroupedRenderItems([before, after], "s1", {
      canAnchorPrepareProgress: false,
    });

    const result = insertLastAgentErrorItem(items, "s1", {
      message: "peer disconnected before response",
      occurredAt: "2026-06-14T10:05:00Z",
    });

    expect(result.map((item) => item.type)).toEqual(["message", "agent_error_notice", "message"]);
  });

  it("uses the notice as the only item when there are no messages", () => {
    const result = insertLastAgentErrorItem([], "s1", {
      message: "peer disconnected before response",
      occurredAt: "2026-06-14T10:05:00Z",
    });

    expect(result).toEqual([
      expect.objectContaining({
        type: "agent_error_notice",
        sessionId: "s1",
      }),
    ]);
  });
});

describe("messageMapsEqualByIdentity", () => {
  it("treats maps with the same keys and identical Message refs as equal", () => {
    const a = makeMessage("perm-1", "permission_request");
    const b = makeMessage("perm-2", "permission_request");
    const first = new Map<string, Message>([
      ["tc-1", a],
      ["tc-2", b],
    ]);
    const second = new Map<string, Message>([
      ["tc-1", a],
      ["tc-2", b],
    ]);
    expect(messageMapsEqualByIdentity(first, second)).toBe(true);
  });

  it("is false when sizes differ", () => {
    const a = makeMessage("perm-1", "permission_request");
    const first = new Map<string, Message>([["tc-1", a]]);
    const second = new Map<string, Message>([
      ["tc-1", a],
      ["tc-2", makeMessage("perm-2", "permission_request")],
    ]);
    expect(messageMapsEqualByIdentity(first, second)).toBe(false);
  });

  it("is false when a key maps to a different Message ref (e.g. status changed)", () => {
    const first = new Map<string, Message>([["tc-1", makeMessage("perm-1", "permission_request")]]);
    const second = new Map<string, Message>([
      ["tc-1", makeMessage("perm-1", "permission_request")],
    ]);
    expect(messageMapsEqualByIdentity(first, second)).toBe(false);
  });
});

describe("messageListMapsEqual", () => {
  it("treats maps whose array values share positional Message refs as equal", () => {
    const c1 = makeMessage("child-1", "tool_call");
    const c2 = makeMessage("child-2", "tool_call");
    const first = new Map<string, Message[]>([["parent-1", [c1, c2]]]);
    const second = new Map<string, Message[]>([["parent-1", [c1, c2]]]);
    expect(messageListMapsEqual(first, second)).toBe(true);
  });

  it("is false when an array length changes (new child appended)", () => {
    const c1 = makeMessage("child-1", "tool_call");
    const first = new Map<string, Message[]>([["parent-1", [c1]]]);
    const second = new Map<string, Message[]>([
      ["parent-1", [c1, makeMessage("child-2", "tool_call")]],
    ]);
    expect(messageListMapsEqual(first, second)).toBe(false);
  });

  it("is false when a child Message ref changes positionally", () => {
    const c1 = makeMessage("child-1", "tool_call");
    const first = new Map<string, Message[]>([["parent-1", [c1]]]);
    const second = new Map<string, Message[]>([
      ["parent-1", [makeMessage("child-1", "tool_call")]],
    ]);
    expect(messageListMapsEqual(first, second)).toBe(false);
  });
});

describe("reconcileRenderItems", () => {
  const turnGroup = (id: string, messages: Message[]): RenderItem => ({
    type: "turn_group",
    id,
    turnId: messages[0]?.turn_id ?? null,
    messages,
  });
  const messageItem = (message: Message): RenderItem => ({ type: "message", message });

  it("returns the prior array unchanged when nothing changed", () => {
    const m1 = makeMessage("m1", "message");
    const prev = [messageItem(m1)];
    const next = [messageItem(m1)];
    expect(reconcileRenderItems(prev, next)).toBe(prev);
  });

  it("reuses unchanged turn-group wrappers while replacing the changed one", () => {
    const a1 = makeMessage("a1", "tool_call");
    const a2 = makeMessage("a2", "tool_call");
    const b1 = makeMessage("b1", "tool_call");
    const b2 = makeMessage("b2", "tool_call");
    const groupA = turnGroup("turn-group-a1", [a1, a2]);
    const groupBPrev = turnGroup("turn-group-b1", [b1]);
    const prev = [groupA, groupBPrev];

    // groupA identical content (rebuilt wrapper); groupB grows by a token.
    const groupBNext = turnGroup("turn-group-b1", [b1, b2]);
    const next = [turnGroup("turn-group-a1", [a1, a2]), groupBNext];

    const result = reconcileRenderItems(prev, next);
    expect(result[0]).toBe(groupA); // stable → memo holds
    expect(result[1]).toBe(groupBNext); // changed → fresh wrapper
  });

  it("keeps stable message wrappers and appends a new item", () => {
    const m1 = makeMessage("m1", "message");
    const prev = [messageItem(m1)];
    const m2 = makeMessage("m2", "message");
    const next = [messageItem(m1), messageItem(m2)];
    const result = reconcileRenderItems(prev, next);
    expect(result).not.toBe(prev);
    expect(result[0]).toBe(prev[0]);
    expect(result).toHaveLength(2);
  });
});
