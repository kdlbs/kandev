import { describe, it, expect, vi } from "vitest";
import {
  parseSlashCommand,
  isKnownCommand,
  emptyTurnNoticeText,
  computeEmptyTurnNotice,
  maybeEmitEmptyTurnNotice,
  type EmptyTurnNoticeInput,
} from "./empty-turn-notice";
import { getBrowserQueryClient } from "@/lib/query/client";
import { qk } from "@/lib/query/keys";
import type { AvailableCommand } from "@/lib/state/slices/session-runtime/types";
import type { Message } from "@/lib/types/http";
import { sessionId, taskId } from "@/lib/types/http";

function userMessage(turnId: string, content: string): Message {
  return {
    id: `user-${turnId}`,
    session_id: sessionId("sess-1"),
    task_id: taskId("task-1"),
    turn_id: turnId,
    author_type: "user",
    content,
    type: "message",
    created_at: "2026-05-30T00:00:00Z",
  };
}

function baseInput(overrides: Partial<EmptyTurnNoticeInput> = {}): EmptyTurnNoticeInput {
  return {
    sessionId: "sess-1",
    taskId: "task-1",
    turnId: "turn-1",
    hadOutput: false,
    isEphemeralSurface: false,
    messages: [userMessage("turn-1", "/pr-fixup please")],
    availableCommands: [],
    now: "2026-05-30T00:00:01Z",
    ...overrides,
  };
}

describe("parseSlashCommand", () => {
  it.each<[string | undefined, string | null]>([
    [undefined, null],
    ["", null],
    ["hello world", null],
    ["/pr-fixup", "pr-fixup"],
    ["  /pr-fixup  ", "pr-fixup"],
    ["/pr-fixup arg1 arg2", "pr-fixup"],
    ["/PR-Fixup", "PR-Fixup"],
    ["/", null],
    ["/   ", null],
  ])("parses %o → %o", (input, expected) => {
    expect(parseSlashCommand(input)).toBe(expected);
  });
});

describe("isKnownCommand", () => {
  const available: AvailableCommand[] = [{ name: "commit" }, { name: "pr-fixup" }];

  it("matches a bare command name", () => {
    expect(isKnownCommand("pr-fixup", available)).toBe(true);
  });
  it("matches case-insensitively", () => {
    expect(isKnownCommand("PR-Fixup", available)).toBe(true);
  });
  it("returns false for an unknown command", () => {
    expect(isKnownCommand("deploy", available)).toBe(false);
  });
  it("returns false when there are no commands", () => {
    expect(isKnownCommand("pr-fixup", undefined)).toBe(false);
    expect(isKnownCommand("pr-fixup", [])).toBe(false);
  });
});

describe("emptyTurnNoticeText", () => {
  it("generic notice for a non-slash prompt", () => {
    expect(emptyTurnNoticeText(null, false)).toBe(
      "The agent finished without producing any output.",
    );
  });
  it("unknown-command notice mentions resending without the slash", () => {
    const text = emptyTurnNoticeText("pr-fixup", false);
    expect(text).toContain("`/pr-fixup` isn't a command this agent recognizes");
    expect(text).toContain("without the leading slash");
  });
  it("known-but-empty notice mentions resending without the slash", () => {
    const text = emptyTurnNoticeText("pr-fixup", true);
    expect(text).toContain("`/pr-fixup` ran but produced no output");
    expect(text).toContain("without the leading slash");
  });
});

describe("computeEmptyTurnNotice", () => {
  it("returns null when the turn produced output", () => {
    expect(computeEmptyTurnNotice(baseInput({ hadOutput: true }))).toBeNull();
  });

  it("returns null when had_output is undefined (older backend)", () => {
    expect(computeEmptyTurnNotice(baseInput({ hadOutput: undefined }))).toBeNull();
  });

  it("returns null on quick-chat / config-chat surfaces", () => {
    expect(computeEmptyTurnNotice(baseInput({ isEphemeralSurface: true }))).toBeNull();
  });

  it("returns null when a notice already exists for this turn (once per turn)", () => {
    const existing: Message = {
      id: "empty-turn-turn-1",
      session_id: sessionId("sess-1"),
      task_id: taskId("task-1"),
      turn_id: "turn-1",
      author_type: "agent",
      content: "x",
      type: "status",
      created_at: "2026-05-30T00:00:00Z",
    };
    const input = baseInput({ messages: [userMessage("turn-1", "/pr-fixup"), existing] });
    expect(computeEmptyTurnNotice(input)).toBeNull();
  });

  it("builds a generic status notice for a non-slash empty turn", () => {
    const notice = computeEmptyTurnNotice(
      baseInput({ messages: [userMessage("turn-1", "do the thing")] }),
    );
    expect(notice).not.toBeNull();
    expect(notice?.id).toBe("empty-turn-turn-1");
    expect(notice?.type).toBe("status");
    expect(notice?.author_type).toBe("agent");
    expect(notice?.turn_id).toBe("turn-1");
    expect(notice?.metadata?.empty_turn).toBe(true);
    expect(notice?.content).toBe("The agent finished without producing any output.");
  });

  it("builds an unknown-command notice", () => {
    const notice = computeEmptyTurnNotice(
      baseInput({ messages: [userMessage("turn-1", "/pr-fixup")], availableCommands: [] }),
    );
    expect(notice?.content).toContain("isn't a command this agent recognizes");
  });

  it("builds a known-but-empty notice when the command is advertised", () => {
    const notice = computeEmptyTurnNotice(
      baseInput({
        messages: [userMessage("turn-1", "/pr-fixup")],
        availableCommands: [{ name: "pr-fixup" }],
      }),
    );
    expect(notice?.content).toContain("ran but produced no output");
  });

  it("uses the most recent user message for the turn", () => {
    const notice = computeEmptyTurnNotice(
      baseInput({
        messages: [
          userMessage("turn-0", "/old"),
          { ...userMessage("turn-1", "/deploy"), id: "u1" },
        ],
      }),
    );
    expect(notice?.content).toContain("`/deploy`");
  });

  // Pins the fix for the "main agent finished, subagent still working" race:
  // backend had_output snapshots before the subagent's nested events land, but
  // the Task tool_call itself is real output for this turn — suppress the notice.
  it("returns null when the turn contains a subagent_task tool call (in any status)", () => {
    const subagentMsg: Message = {
      id: "tool-1",
      session_id: sessionId("sess-1"),
      task_id: taskId("task-1"),
      turn_id: "turn-1",
      author_type: "agent",
      content: "Investigate",
      type: "tool_call",
      metadata: { normalized: { kind: "subagent_task" }, status: "running" },
      created_at: "2026-05-30T00:00:00Z",
    };
    const input = baseInput({
      messages: [userMessage("turn-1", "spawn a subagent"), subagentMsg],
    });
    expect(computeEmptyTurnNotice(input)).toBeNull();
  });
});

describe("maybeEmitEmptyTurnNotice", () => {
  it("reads advertised slash commands from the TanStack Query cache", () => {
    const queryClient = getBrowserQueryClient();
    queryClient.clear();
    queryClient.setQueryData(qk.sessionRuntime.availableCommands("sess-1"), [{ name: "pr-fixup" }]);
    const addMessage = vi.fn();
    const store = {
      getState: () => ({
        quickChat: { sessions: [] },
        configChat: { sessions: [] },
        messages: { bySession: { "sess-1": [userMessage("turn-1", "/pr-fixup")] } },
        addMessage,
      }),
    };

    maybeEmitEmptyTurnNotice(
      store as never,
      {
        id: "turn-1",
        session_id: "sess-1",
        task_id: "task-1",
        had_output: false,
      } as never,
      "2026-05-30T00:00:01Z",
    );

    expect(addMessage).toHaveBeenCalledWith(
      expect.objectContaining({ content: expect.stringContaining("ran but produced no output") }),
    );
  });
});
