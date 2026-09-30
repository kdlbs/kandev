import type { ImmerSet } from "./session-runtime-slice";
import type { SessionRuntimeSlice } from "./types";

type SessionViewActions = Pick<
  SessionRuntimeSlice,
  | "setAgentCapabilities"
  | "setSessionModels"
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
