import { describe, expect, it } from "vitest";
import type { Message } from "@/lib/types/http";
import type { RenderItem } from "@/hooks/use-processed-messages";
import {
  buildActivityItems,
  deriveRunningTurn,
  deriveStatusLine,
  formatActivityDuration,
} from "./activity-display";

let seq = 0;
function msg(over: Partial<Message> & { meta?: Record<string, unknown> }): Message {
  seq++;
  const { meta, ...rest } = over;
  return {
    id: `m${seq}`,
    session_id: "s1",
    task_id: "t1",
    author_type: "agent",
    content: "",
    type: "tool_call",
    created_at: `2026-09-29T10:00:${String(seq % 60).padStart(2, "0")}Z`,
    metadata: meta,
    ...rest,
  } as Message;
}
const tool = (turn: string, name: string, status = "complete", at?: string, extra = {}) =>
  msg({
    turn_id: turn,
    created_at: at ?? `2026-09-29T10:00:${String(seq % 60).padStart(2, "0")}Z`,
    meta: { status, normalized: { generic: { name: `mcp__kandev__${name}_kandev` } }, ...extra },
  });
const asItems = (messages: Message[]): RenderItem[] =>
  messages.map((message) => ({ type: "message", message }));
const idle = { turnId: null, running: false, waiting: false, awaiting: new Set<string>() };

describe("formatActivityDuration", () => {
  it("formats seconds, minutes and hours with a 1s minimum", () => {
    expect(formatActivityDuration(0)).toBe("1s");
    expect(formatActivityDuration(59.9)).toBe("59s");
    expect(formatActivityDuration(65)).toBe("1m 5s");
    expect(formatActivityDuration(3660)).toBe("1h 1m");
  });
});

describe("deriveRunningTurn", () => {
  const messages = [msg({ turn_id: "old" }), msg({ turn_id: "new", type: "message" })];
  it("uses the active id, else the latest loaded message's turn id", () => {
    expect(deriveRunningTurn({ messages, sessionState: "RUNNING", activeTurnId: "x" }).turnId).toBe(
      "x",
    );
    expect(
      deriveRunningTurn({ messages, sessionState: "RUNNING", activeTurnId: null }).turnId,
    ).toBe("new");
  });
  it("is not running for settled states", () => {
    expect(deriveRunningTurn({ messages, sessionState: "IDLE", activeTurnId: "x" }).running).toBe(
      false,
    );
    expect(
      deriveRunningTurn({ messages, sessionState: "STARTING", activeTurnId: null }).running,
    ).toBe(false);
  });
  it("keeps a waiting session running only while a permission of the turn is pending", () => {
    const req = (turn: string, status?: string) =>
      msg({
        type: "permission_request",
        turn_id: turn,
        meta: { tool_call_id: "c1", ...(status ? { status } : {}) },
      });
    const waiting = deriveRunningTurn({
      messages: [...messages, req("new")],
      sessionState: "WAITING_FOR_INPUT",
      activeTurnId: null,
    });
    expect(waiting).toMatchObject({ running: true, waiting: true });
    expect(waiting.awaiting.has("c1")).toBe(true);
    const stale = deriveRunningTurn({
      messages: [req("old"), ...messages.slice(1)],
      sessionState: "WAITING_FOR_INPUT",
      activeTurnId: null,
    });
    expect(stale.running).toBe(false);
    const decided = deriveRunningTurn({
      messages: [...messages, req("new", "approved")],
      sessionState: "WAITING_FOR_INPUT",
      activeTurnId: null,
    });
    expect(decided.running).toBe(false);
  });
});

describe("buildActivityItems", () => {
  it("collapses a finished turn's calls into one chip with count, failed and duration", () => {
    const a = tool("t", "list_tasks", "complete", "2026-09-29T10:00:00Z");
    const text = msg({ turn_id: "t", type: "message", content: "hi" });
    const b = tool("t", "list_workflows", "failed", "2026-09-29T10:00:09Z");
    const out = buildActivityItems(asItems([a, text, b]), idle);
    expect(out).toHaveLength(2);
    expect(out[0]).toMatchObject({
      type: "turn_group",
      activityChip: { count: 2, failed: 1, durationSeconds: 9 },
    });
    expect(out[1]).toMatchObject({ type: "message" });
  });

  it("keeps a propose_task call out of the chip", () => {
    const a = tool("t", "list_tasks");
    const p = tool("t", "propose_task");
    const out = buildActivityItems(asItems([a, p]), idle);
    expect(out.map((i) => i.type)).toEqual(["turn_group", "message"]);
    expect((out[0] as { messages: Message[] }).messages).toEqual([a]);
  });

  it("gives one call a chip of one and no calls no chip", () => {
    const one = buildActivityItems(asItems([tool("t", "list_tasks")]), idle);
    expect(one[0]).toMatchObject({ activityChip: { count: 1 } });
    const thinkingOnly = msg({ turn_id: "t", type: "thinking" });
    const none = buildActivityItems(asItems([thinkingOnly, tool("t", "propose_task")]), idle);
    expect(none.every((i) => i.type === "message")).toBe(true);
  });

  it("omits the duration when no timestamp is valid", () => {
    const a = tool("t", "list_tasks", "complete", "garbage");
    const out = buildActivityItems(asItems([a]), idle);
    expect(out[0]).toMatchObject({ activityChip: { durationSeconds: null } });
  });

  it("starts a further chip after a user message inside the turn", () => {
    const user = msg({ turn_id: "t", author_type: "user", type: "message" });
    const out = buildActivityItems(
      asItems([tool("t", "list_tasks"), user, tool("t", "list_workflows")]),
      idle,
    );
    expect(out.map((i) => (i.type === "turn_group" ? i.id : "m"))).toEqual([
      "activity-chip-t-0",
      "m",
      "activity-chip-t-1",
    ]);
  });

  describe("while the turn runs", () => {
    const running = { turnId: "t", running: true, waiting: false, awaiting: new Set<string>() };
    it("hides the running turn's activity but not an older turn's chip", () => {
      const out = buildActivityItems(
        asItems([tool("old", "list_tasks"), tool("t", "list_tasks", "running")]),
        running,
      );
      expect(out).toHaveLength(1);
      expect(out[0]).toMatchObject({ type: "turn_group", turnId: "old" });
    });

    it("keeps a call awaiting permission visible", () => {
      const awaiting = tool("t", "list_tasks", "pending", undefined, { tool_call_id: "c1" });
      const out = buildActivityItems(asItems([awaiting, tool("t", "list_workflows")]), {
        ...running,
        awaiting: new Set(["c1"]),
      });
      expect(out).toEqual([{ type: "message", message: awaiting }]);
    });

    it("shows a returned proposal but hides an unreturned or failed one", () => {
      const ok = tool("t", "propose_task", "complete", undefined, {
        normalized: {
          generic: {
            name: "mcp__kandev__propose_task_kandev",
            output: JSON.stringify({ proposal_id: "p1" }),
          },
        },
      });
      const unreturned = tool("t", "propose_task", "running");
      const failed = tool("t", "propose_task", "failed");
      expect(buildActivityItems(asItems([ok]), running)).toHaveLength(1);
      expect(buildActivityItems(asItems([unreturned, failed]), running)).toHaveLength(0);
      expect(buildActivityItems(asItems([unreturned]), idle)).toHaveLength(1);
    });

    it("hides nothing when the turn id is unknown", () => {
      const out = buildActivityItems(asItems([tool("t", "list_tasks", "running")]), {
        ...running,
        turnId: null,
      });
      expect(out).toHaveLength(1);
    });
  });
});

describe("copilot proposal visibility", () => {
  it.each(["propose_task", "propose_resume", "propose_message", "propose_move"])(
    "keeps a returned %s proposal card visible in the copilot",
    (kind) => {
      const proposal = tool("t", kind, "complete", undefined, {
        normalized: {
          generic: {
            name: `mcp__kandev__${kind}_kandev`,
            output: JSON.stringify({ proposal_id: `proposal-${kind}` }),
          },
        },
      });
      const running = {
        turnId: "t",
        running: true,
        waiting: false,
        awaiting: new Set<string>(),
      };
      const items = asItems([proposal]);
      expect(buildActivityItems(items, running)).toEqual([{ type: "message", message: proposal }]);
      expect(buildActivityItems(items, idle)).toEqual([{ type: "message", message: proposal }]);
    },
  );
});

describe("deriveStatusLine", () => {
  const running = { turnId: "t", running: true, waiting: false, awaiting: new Set<string>() };
  it("is absent when nothing runs", () => {
    expect(deriveStatusLine([], idle)).toBeNull();
  });
  it("names the latest running Kandev tool, falling back to Working", () => {
    const done = tool("t", "list_workflows", "complete");
    const live = tool("t", "list_tasks", "running");
    expect(deriveStatusLine([done, live], running)?.verbKey).toBe(
      "coordinator:activityVerbListTasks",
    );
    expect(deriveStatusLine([done], running)?.verbKey).toBe("coordinator:activityVerbWorking");
    const unknown = tool("t", "something_else", "running");
    expect(deriveStatusLine([unknown], running)?.verbKey).toBe("coordinator:activityVerbWorking");
    const foreign = msg({ turn_id: "t", meta: { status: "running" } });
    expect(deriveStatusLine([foreign], running)?.verbKey).toBe("coordinator:activityVerbWorking");
  });
  it("says waiting for a decision while only a permission holds the turn", () => {
    expect(deriveStatusLine([], { ...running, waiting: true })?.verbKey).toBe(
      "coordinator:activityVerbWaiting",
    );
  });
  it("counts from the turn's first message", () => {
    const first = msg({ turn_id: "t", created_at: "2026-09-29T10:00:00Z", type: "message" });
    expect(deriveStatusLine([first], running)?.startedAtMs).toBe(
      Date.parse("2026-09-29T10:00:00Z"),
    );
    const bad = msg({ turn_id: "t", created_at: "nope", type: "message" });
    expect(deriveStatusLine([bad], running)?.startedAtMs).toBeNull();
  });
});
