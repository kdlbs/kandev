import { describe, expect, it } from "vitest";
import {
  sessionId as toSessionId,
  taskId as toTaskId,
  type Message,
  type MessageType,
} from "@/lib/types/http";
import { buildGroupedRenderItems, insertLastAgentErrorItem } from "./use-processed-messages";

const SESSION_ID = "s1";
const ERROR_OCCURRED_AT = "2026-06-14T10:05:00Z";

function makeMessage(
  id: string,
  type: MessageType,
  metadata?: Record<string, unknown>,
  session = SESSION_ID,
): Message {
  return {
    id,
    session_id: toSessionId(session),
    task_id: toTaskId("t1"),
    author_type: "agent",
    content: "The agent encountered an error.",
    type,
    metadata,
    created_at: ERROR_OCCURRED_AT,
  };
}

describe("insertLastAgentErrorItem recovery identity", () => {
  it("keeps one representation when the persisted recovery message is present", () => {
    const persisted = makeMessage("recovery-1", "status", {
      recovery_actions: true,
      error_stamp: "failure-1",
    });
    const items = buildGroupedRenderItems([persisted], SESSION_ID, {
      canAnchorPrepareProgress: false,
    });

    const result = insertLastAgentErrorItem(
      items,
      SESSION_ID,
      { message: persisted.content, occurredAt: ERROR_OCCURRED_AT, stamp: "failure-1" },
      [persisted],
    );

    expect(result.map((item) => item.type)).toEqual(["message"]);
    expect(result.filter((item) => item.type === "agent_error_notice")).toHaveLength(0);
  });

  it.each([
    {
      name: "a different failure stamp",
      messageSession: SESSION_ID,
      messageStamp: "failure-1",
      errorStamp: "failure-2",
    },
    {
      name: "a different session",
      messageSession: "s2",
      messageStamp: "failure-1",
      errorStamp: "failure-1",
    },
  ])("keeps the provisional notice for $name", ({ messageSession, messageStamp, errorStamp }) => {
    const persisted = makeMessage(
      "recovery-1",
      "status",
      { recovery_actions: true, error_stamp: messageStamp },
      messageSession,
    );
    const items = buildGroupedRenderItems([], SESSION_ID, {
      canAnchorPrepareProgress: false,
    });

    const result = insertLastAgentErrorItem(
      items,
      SESSION_ID,
      { message: "new failure", occurredAt: ERROR_OCCURRED_AT, stamp: errorStamp },
      [persisted],
    );

    expect(result.map((item) => item.type)).toEqual(["agent_error_notice"]);
  });
});
