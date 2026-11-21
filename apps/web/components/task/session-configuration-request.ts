import type { MutableRefObject } from "react";

type SessionExecutionState = {
  taskSessions: {
    items: Record<string, { agent_execution_id?: string } | undefined>;
  };
  sessionModels: {
    bySessionId: Record<string, { confirmedConfigOptionsExecutionId?: string } | undefined>;
  };
};

type SessionConfigurationStore = {
  getState: () => SessionExecutionState;
};

export function sessionConfigurationExecutionId(state: SessionExecutionState, sessionId: string) {
  return (
    state.taskSessions.items[sessionId]?.agent_execution_id ??
    state.sessionModels.bySessionId[sessionId]?.confirmedConfigOptionsExecutionId
  );
}

type ConfigurationResponse = {
  configuration_result?: {
    status?: string;
    confirmed?: boolean;
    effective?: string;
    execution_id?: string;
  };
};

function configurationResponse(value: unknown): ConfigurationResponse | undefined {
  if (!value || typeof value !== "object") return undefined;
  const response = value as ConfigurationResponse & { body?: unknown };
  if (response.configuration_result) return response;
  if (response.body && typeof response.body === "object") {
    return response.body as ConfigurationResponse;
  }
  return undefined;
}

export function createSessionConfigurationRequestFence({
  sessionId,
  store,
  latestRequestIds,
  requestId,
}: {
  sessionId: string;
  store: SessionConfigurationStore;
  latestRequestIds: MutableRefObject<Record<string, number>>;
  requestId: number;
}) {
  const sessionExecutionId = () => sessionConfigurationExecutionId(store.getState(), sessionId);
  const initialExecutionId = sessionExecutionId();
  let responseExecutionId: string | undefined;
  let mismatchedResponse = false;

  const isCurrent = () => {
    if (mismatchedResponse || latestRequestIds.current[sessionId] !== requestId) {
      return false;
    }
    const expectedExecutionId = initialExecutionId ?? responseExecutionId;
    if (!expectedExecutionId) return true;
    return sessionExecutionId() === expectedExecutionId;
  };

  const observeResponse = (response: unknown) => {
    if (!isCurrent()) return false;
    const result = configurationResponse(response)?.configuration_result;
    const resultExecutionId = result?.execution_id;
    if (!resultExecutionId) return true;
    if (initialExecutionId && resultExecutionId !== initialExecutionId) {
      mismatchedResponse = true;
      return false;
    }
    const currentExecutionId = sessionExecutionId();
    if (currentExecutionId && currentExecutionId !== resultExecutionId) {
      mismatchedResponse = true;
      return false;
    }
    responseExecutionId = resultExecutionId;
    return true;
  };

  const canCommit = (response: unknown) => {
    if (!isCurrent()) return false;
    const result = configurationResponse(response)?.configuration_result;
    const resultExecutionId = result?.execution_id;
    const expectedExecutionId = initialExecutionId ?? responseExecutionId;
    return (
      (result?.status === "applied" || result?.status === "persistence_failed") &&
      result.confirmed === true &&
      typeof result.effective === "string" &&
      result.effective.length > 0 &&
      !!expectedExecutionId &&
      resultExecutionId === expectedExecutionId &&
      sessionExecutionId() === expectedExecutionId
    );
  };

  return { isCurrent, observeResponse, canCommit };
}
