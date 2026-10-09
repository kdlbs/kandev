import { describe, expect, it } from "vitest";
import { profileStatusMessages } from "./profile-messages";

const t = (key: string) => key;

describe("profileStatusMessages", () => {
  it("returns the agent missing message and no executor message", () => {
    expect(profileStatusMessages("missing", "ok", t)).toEqual({
      agentMessage: "coordinator:copilotAgentMissingMessage",
      executorMessage: null,
    });
  });

  it("returns the agent passthrough message, not the missing one", () => {
    expect(profileStatusMessages("passthrough", "ok", t)).toEqual({
      agentMessage: "coordinator:copilotAgentPassthroughMessage",
      executorMessage: null,
    });
  });

  it("returns the executor missing message and no agent message", () => {
    expect(profileStatusMessages("ok", "missing", t)).toEqual({
      agentMessage: null,
      executorMessage: "coordinator:copilotExecutorMissingMessage",
    });
  });

  it("returns both messages when both are not ok", () => {
    expect(profileStatusMessages("passthrough", "missing", t)).toEqual({
      agentMessage: "coordinator:copilotAgentPassthroughMessage",
      executorMessage: "coordinator:copilotExecutorMissingMessage",
    });
  });

  it("returns no messages when both are ok", () => {
    expect(profileStatusMessages("ok", "ok", t)).toEqual({
      agentMessage: null,
      executorMessage: null,
    });
  });

  it("shows an unknown agent status as the agent missing message", () => {
    expect(profileStatusMessages("unexpected", "ok", t)).toEqual({
      agentMessage: "coordinator:copilotAgentMissingMessage",
      executorMessage: null,
    });
  });

  it("shows an unknown executor status as the executor missing message", () => {
    expect(profileStatusMessages("ok", "unexpected", t)).toEqual({
      agentMessage: null,
      executorMessage: "coordinator:copilotExecutorMissingMessage",
    });
  });
});
