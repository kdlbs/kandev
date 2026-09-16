import { describe, expect, it } from "vitest";
import { sessionId as toSessionId, taskId as toTaskId, type Message } from "@/lib/types/http";
import { buildGroupedRenderItems, insertLastAgentErrorItem } from "./use-processed-messages";

const ERROR_MESSAGE = "peer disconnected before response";
const ERROR_OCCURRED_AT = "2026-06-14T10:05:00Z";

function legacyRecoveryMessage(): Message {
  return {
    id: "legacy-recovery",
    session_id: toSessionId("s1"),
    task_id: toTaskId("t1"),
    author_type: "agent",
    content: `Agent startup failed: ${ERROR_MESSAGE}`,
    type: "status",
    metadata: { recovery_actions: true },
    created_at: ERROR_OCCURRED_AT,
  };
}

describe("legacy recovery message deduplication", () => {
  it("uses a matching unstamped row instead of adding a provisional notice", () => {
    const legacy = legacyRecoveryMessage();
    const items = buildGroupedRenderItems([legacy], "s1");

    const result = insertLastAgentErrorItem(
      items,
      "s1",
      { message: ERROR_MESSAGE, occurredAt: ERROR_OCCURRED_AT },
      [legacy],
    );

    expect(result).toBe(items);
  });
});
