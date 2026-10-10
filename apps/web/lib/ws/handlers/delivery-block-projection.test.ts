import { describe, it, expect } from "vitest";
import { createAppStore } from "@/lib/state/store";
import { registerTaskSessionHandlers } from "./agent-session";
import type { BackendMessageMap } from "@/lib/types/backend";

describe("delivery block projection", () => {
  it("applies and clears live blocks, preserves omitted fields, and rejects older snapshots", () => {
    const store = createAppStore();
    const handler = registerTaskSessionHandlers(store)["session.state_changed"]!;
    const send = (at: string, fields: Record<string, unknown>) =>
      handler({
        id: at,
        type: "notification",
        action: "session.state_changed",
        payload: {
          task_id: "task",
          session_id: "session",
          new_state: "WAITING_FOR_INPUT",
          updated_at: at,
          ...fields,
        },
      } as BackendMessageMap["session.state_changed"]);
    const blocks = [
      {
        id: "block",
        incarnation_id: "inc",
        expected_generation: 1,
        reason: "unresolved_durable_work",
        consumer_reference: "agent_delivery",
        updated_at: "2026-10-10T00:00:01.000001Z",
      },
    ];
    send("2026-10-10T00:00:01.000001Z", { session_recovery_blocks: blocks });
    expect(store.getState().taskSessions.items.session.session_recovery_blocks).toEqual(blocks);
    send("2026-10-10T00:00:01.000002Z", {});
    expect(store.getState().taskSessions.items.session.session_recovery_blocks).toEqual(blocks);
    send("2026-10-10T00:00:01.000003Z", { session_recovery_blocks: [] });
    send("2026-10-10T00:00:01.000001Z", { session_recovery_blocks: blocks });
    expect(store.getState().taskSessions.items.session.session_recovery_blocks).toEqual([]);
  });
});
