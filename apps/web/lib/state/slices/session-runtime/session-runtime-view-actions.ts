import type { ImmerSet } from "./session-runtime-slice";
import type { SessionRuntimeSlice } from "./types";

type SessionViewActions = Pick<
  SessionRuntimeSlice,
  | "setAgentCapabilities"
  | "setSessionModels"
  | "invalidateConfirmedConfigOptions"
  | "setEmbeddedVscodeSupport"
  | "setSessionMCPStatus"
  | "setPromptUsage"
  | "bumpSessionUsageInvalidation"
  | "setSessionTodos"
  | "setLaunchWarning"
  | "clearLaunchWarning"
>;

export function buildSessionViewActions(set: ImmerSet): SessionViewActions {
  return {
    setAgentCapabilities: (sessionId, caps) =>
      set((draft) => {
        draft.agentCapabilities.bySessionId[sessionId] = caps;
      }),
    setSessionModels: (sessionId, data) =>
      set((draft) => {
        draft.sessionModels.bySessionId[sessionId] = data;
      }),
    invalidateConfirmedConfigOptions: (sessionId, executionId) =>
      set((draft) => {
        const models = draft.sessionModels.bySessionId[sessionId];
        if (!models) return;

        if (executionId) {
          if (models.confirmedConfigOptionsExecutionId === executionId) {
            delete models.pendingConfirmedConfigOptions;
            return;
          }

          const pending = models.pendingConfirmedConfigOptions;
          if (pending?.executionId === executionId) {
            models.confirmedConfigOptions = pending.values;
            models.confirmedConfigOptionsExecutionId = executionId;
          } else {
            delete models.confirmedConfigOptions;
            delete models.confirmedConfigOptionsExecutionId;
          }
          delete models.pendingConfirmedConfigOptions;
          return;
        }

        if (
          models.confirmedConfigOptions !== undefined &&
          models.confirmedConfigOptionsExecutionId
        ) {
          models.pendingConfirmedConfigOptions = {
            executionId: models.confirmedConfigOptionsExecutionId,
            values: models.confirmedConfigOptions,
          };
        }
        delete models.confirmedConfigOptions;
        delete models.confirmedConfigOptionsExecutionId;
      }),
    setEmbeddedVscodeSupport: (sessionId, supported) =>
      set((draft) => {
        draft.embeddedVscodeSupport.bySessionId[sessionId] = supported;
      }),
    setSessionMCPStatus: (sessionId, history) =>
      set((draft) => {
        draft.sessionMcpStatus.bySessionId[sessionId] = history;
      }),
    setPromptUsage: (sessionId, usage) =>
      set((draft) => {
        draft.promptUsage.bySessionId[sessionId] = usage;
      }),
    bumpSessionUsageInvalidation: (sessionId) =>
      set((draft) => {
        draft.usageInvalidation.bySessionId[sessionId] =
          (draft.usageInvalidation.bySessionId[sessionId] ?? 0) + 1;
      }),
    setSessionTodos: (sessionId, entries) =>
      set((draft) => {
        draft.sessionTodos.bySessionId[sessionId] = entries;
      }),
    setLaunchWarning: (sessionId, entry) =>
      set((draft) => {
        draft.launchWarning.bySessionId[sessionId] = entry;
      }),
    clearLaunchWarning: (sessionId) =>
      set((draft) => {
        delete draft.launchWarning.bySessionId[sessionId];
      }),
  };
}
