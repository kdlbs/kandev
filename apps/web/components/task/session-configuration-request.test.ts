import { describe, expect, it } from "vitest";

import { createSessionConfigurationRequestFence } from "./session-configuration-request";

const EXECUTION_1 = "execution-1";
const EXECUTION_2 = "execution-2";
const MODEL_B = "model-b";

function fixture(executionId: string | undefined) {
  const state = {
    taskSessions: { items: { session: { agent_execution_id: executionId } } },
    sessionModels: { bySessionId: { session: {} } },
  };
  const latestRequestIds = { current: { session: 1 } };
  const store = { getState: () => state };
  return {
    state,
    latestRequestIds,
    fence: createSessionConfigurationRequestFence({
      sessionId: "session",
      store,
      latestRequestIds,
      requestId: 1,
    }),
  };
}

describe("session configuration request fence", () => {
  it("rejects a result from a replaced execution", () => {
    const { state, fence } = fixture(EXECUTION_1);
    state.taskSessions.items.session.agent_execution_id = EXECUTION_2;

    expect(
      fence.observeResponse({
        configuration_result: {
          status: "applied",
          confirmed: true,
          effective: MODEL_B,
          execution_id: EXECUTION_1,
        },
      }),
    ).toBe(false);
    expect(fence.isCurrent()).toBe(false);
  });

  it("requires matching live and returned execution identities before state writes", () => {
    const { fence } = fixture(EXECUTION_2);
    const response = {
      configuration_result: {
        status: "applied",
        confirmed: true,
        effective: MODEL_B,
        execution_id: EXECUTION_1,
      },
    };

    expect(fence.observeResponse(response)).toBe(false);
    expect(fence.canCommit(response)).toBe(false);
  });

  it("uses the returned identity only when the live session reports that execution", () => {
    const { state, fence } = fixture(undefined);
    const response = {
      configuration_result: {
        status: "applied",
        confirmed: true,
        effective: MODEL_B,
        execution_id: EXECUTION_1,
      },
    };

    expect(fence.observeResponse(response)).toBe(true);
    expect(fence.canCommit(response)).toBe(false);
    state.taskSessions.items.session.agent_execution_id = EXECUTION_1;
    expect(fence.isCurrent()).toBe(true);
    expect(fence.canCommit(response)).toBe(true);
  });
});
