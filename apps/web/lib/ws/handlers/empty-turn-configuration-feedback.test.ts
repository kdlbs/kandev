import { describe, expect, it } from "vitest";
import { computeEmptyTurnNotice } from "./empty-turn-notice";
import type { EmptyTurnNoticeInput } from "./empty-turn-notice";
import type { ConfigurationCommandResult } from "@/lib/types/backend";
import type { Message } from "@/lib/types/http";
import { sessionId, taskId } from "@/lib/types/http";

const REQUESTED_MODEL = "mock-smart";
const AUTO_MODEL = "Auto";
const FUSION_HIGH = "Fusion High";

function commandMessage(): Message {
  return {
    id: "user-turn-1",
    session_id: sessionId("session-1"),
    task_id: taskId("task-1"),
    turn_id: "turn-1",
    author_type: "user",
    content: `/model ${REQUESTED_MODEL}`,
    type: "message",
    created_at: "2026-05-30T00:00:00Z",
  };
}

function notice(result: ConfigurationCommandResult): Message | null {
  const input: EmptyTurnNoticeInput = {
    sessionId: "session-1",
    taskId: "task-1",
    turnId: "turn-1",
    hadOutput: false,
    isEphemeralSurface: false,
    messages: [commandMessage()],
    availableCommands: [],
    configuration_result: result,
    now: "2026-05-30T00:00:01Z",
  };
  return computeEmptyTurnNotice(input);
}

describe("configuration command empty-turn feedback", () => {
  it("uses the correlated result instead of the generic empty-turn hint", () => {
    const result: ConfigurationCommandResult = {
      status: "applied",
      config_id: "model",
      requested: REQUESTED_MODEL,
      effective: REQUESTED_MODEL,
      confirmed: true,
    };

    const message = notice(result);

    expect(message?.content).toContain(REQUESTED_MODEL);
    expect(message?.content).not.toContain("finished without producing");
    expect(message?.metadata?.configuration_command_feedback).toBe(true);
    expect(message?.metadata?.feedback_status).toBe("applied");
  });

  it("shows partial provider evidence as dismissible unconfirmed feedback", () => {
    const message = notice({
      status: "unconfirmed",
      config_id: "model",
      requested: FUSION_HIGH,
      effective: AUTO_MODEL,
      confirmed: false,
    });

    expect(message?.content).toContain(AUTO_MODEL);
    expect(message?.content).toContain(FUSION_HIGH);
    expect(message?.content).toMatch(/confirm/i);
    expect(message?.metadata?.feedback_status).toBe("unconfirmed");
    expect(message?.metadata?.dismissible).toBe(true);
  });

  it("shows a brief clamped receipt when Copilot reaches Auto but not the requested tier", () => {
    const message = notice({
      status: "clamped",
      config_id: "model",
      requested: FUSION_HIGH,
      effective: AUTO_MODEL,
      confirmed: false,
    });

    expect(message?.content).toContain(FUSION_HIGH);
    expect(message?.content).toContain(AUTO_MODEL);
    expect(message?.metadata?.feedback_status).toBe("clamped");
    expect(message?.metadata?.dismissible).toBe(false);
  });

  it("downgrades an unconfirmed applied result to dismissible feedback", () => {
    const message = notice({
      status: "applied",
      config_id: "model",
      requested: REQUESTED_MODEL,
      effective: AUTO_MODEL,
      confirmed: false,
    });

    expect(message?.metadata?.feedback_status).toBe("unconfirmed");
    expect(message?.metadata?.dismissible).toBe(true);
    expect(message?.content).toContain(AUTO_MODEL);
    expect(message?.content).toContain(REQUESTED_MODEL);
  });
});
