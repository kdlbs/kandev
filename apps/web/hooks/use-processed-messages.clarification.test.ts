import { describe, it, expect } from "vitest";
import {
  sessionId as toSessionId,
  taskId as toTaskId,
  type Message,
  type MessageType,
} from "@/lib/types/http";
import {
  findPendingClarification,
  findPendingClarificationGroup,
} from "@/lib/utils/pending-clarification";
import { filterVisibleMessages } from "./processed-message-filtering";

const CURRENT_TURN_ID = "turn-current";
const OLD_QUESTION_ID = "old-question";
const CURRENT_QUESTION_ID = "current-question";
const CLARIFICATION_REQUEST: MessageType = "clarification_request";

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

describe("filterVisibleMessages clarification history", () => {
  it("uses the newest pending clarification as the active fallback when scope is absent", () => {
    const old = makeMessage("fallback-old-question", CLARIFICATION_REQUEST, {
      status: "pending",
    });
    const active = makeMessage("fallback-active-question", CLARIFICATION_REQUEST, {
      status: "pending",
    });

    expect(
      filterVisibleMessages([old, active], new Set<string>(), new Set<string>()).map(
        (message) => message.id,
      ),
    ).toEqual(["fallback-old-question"]);
  });

  it("keeps a superseded pending clarification as inert transcript history", () => {
    const superseded = {
      ...makeMessage(OLD_QUESTION_ID, CLARIFICATION_REQUEST, {
        status: "pending",
        agent_disconnected: true,
      }),
      turn_id: "turn-old",
    };
    const current = {
      ...makeMessage(CURRENT_QUESTION_ID, CLARIFICATION_REQUEST, { status: "pending" }),
      turn_id: CURRENT_TURN_ID,
    };

    expect(
      filterVisibleMessages([superseded, current], new Set<string>(), new Set<string>(), {
        currentTurnId: CURRENT_TURN_ID,
      }).map((message) => message.id),
    ).toEqual([OLD_QUESTION_ID]);
  });
});

describe("filterVisibleMessages superseded detached clarification", () => {
  it("keeps the older detached bundle in history when the current bundle resolves", () => {
    const superseded = {
      ...makeMessage(OLD_QUESTION_ID, CLARIFICATION_REQUEST, {
        pending_id: "pending-old",
        status: "pending",
        agent_disconnected: true,
      }),
      turn_id: "turn-old",
    };
    const current = {
      ...makeMessage(CURRENT_QUESTION_ID, CLARIFICATION_REQUEST, {
        pending_id: "pending-current",
        status: "answered",
      }),
      turn_id: CURRENT_TURN_ID,
    };
    const activity = {
      ...makeMessage("current-activity", "message", undefined, "Continuing with the new turn."),
      turn_id: CURRENT_TURN_ID,
    };
    const scope = {
      currentTurnId: CURRENT_TURN_ID,
      currentTurnCompleted: false,
      pendingAction: null,
    } as const;

    expect(
      filterVisibleMessages([superseded, current, activity], new Set<string>(), new Set<string>(), {
        ...scope,
      }).map((message) => message.id),
    ).toEqual([OLD_QUESTION_ID, CURRENT_QUESTION_ID, "current-activity"]);
    expect(findPendingClarification([superseded, current, activity], scope)).toBeNull();
    expect(findPendingClarificationGroup([superseded, current, activity], scope)).toEqual([]);
  });
});

describe("filterVisibleMessages current-turn clarification bundles", () => {
  it("hides only the newest pending clarification bundle in the current turn", () => {
    const superseded = {
      ...makeMessage("same-turn-old-question", CLARIFICATION_REQUEST, {
        status: "pending",
        pending_id: "pending-old",
      }),
      turn_id: CURRENT_TURN_ID,
    };
    const active = {
      ...makeMessage("same-turn-active-question", CLARIFICATION_REQUEST, {
        status: "pending",
        pending_id: "pending-active",
      }),
      turn_id: CURRENT_TURN_ID,
    };

    expect(
      filterVisibleMessages([superseded, active], new Set<string>(), new Set<string>(), {
        currentTurnId: CURRENT_TURN_ID,
      }).map((message) => message.id),
    ).toEqual(["same-turn-old-question"]);
  });
});

describe("filterVisibleMessages clarification history without turn authority", () => {
  it("keeps malformed pending history visible when durable turn history is empty", () => {
    const pending = makeMessage("legacy-question", CLARIFICATION_REQUEST, {
      status: "pending",
    });

    expect(
      filterVisibleMessages([pending], new Set<string>(), new Set<string>(), {
        currentTurnId: null,
      }).map((message) => message.id),
    ).toEqual(["legacy-question"]);
    expect(
      findPendingClarificationGroup([pending], { currentTurnId: null }).map(
        (message) => message.id,
      ),
    ).toEqual([]);
  });

  it.each([null, "permission"] as const)(
    "keeps pending metadata as inert history when authority is %s",
    (pendingAction) => {
      const old = makeMessage("unhydrated-old-question", CLARIFICATION_REQUEST, {
        status: "pending",
        agent_disconnected: true,
      });

      expect(
        filterVisibleMessages([old], new Set<string>(), new Set<string>(), {
          currentTurnId: undefined,
          pendingAction,
        }).map((message) => message.id),
      ).toEqual(["unhydrated-old-question"]);
    },
  );

  it("keeps pending clarification as inert history when explicit authority is unavailable", () => {
    const pending = makeMessage("unloaded-question", CLARIFICATION_REQUEST, {
      status: "pending",
    });

    expect(
      filterVisibleMessages([pending], new Set<string>(), new Set<string>(), {
        currentTurnId: undefined,
        pendingAction: undefined,
      }).map((message) => message.id),
    ).toEqual(["unloaded-question"]);
  });

  it("hides only the active bundle when clarification authority exists before turn hydration", () => {
    const old = makeMessage("unhydrated-superseded", CLARIFICATION_REQUEST, {
      status: "pending",
      pending_id: "pending-old",
    });
    const activeA = makeMessage("unhydrated-active-a", CLARIFICATION_REQUEST, {
      status: "pending",
      pending_id: "pending-active",
    });
    const activeB = makeMessage("unhydrated-active-b", CLARIFICATION_REQUEST, {
      status: "pending",
      pending_id: "pending-active",
    });

    expect(
      filterVisibleMessages([old, activeA, activeB], new Set<string>(), new Set<string>(), {
        currentTurnId: undefined,
        pendingAction: "clarification",
      }).map((message) => message.id),
    ).toEqual(["unhydrated-superseded"]);
  });
});
