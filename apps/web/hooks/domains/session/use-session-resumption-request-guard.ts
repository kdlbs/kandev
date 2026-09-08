import type { RefObject } from "react";
import type { ResumeStateSetter } from "./use-session-resumption";

export type SessionRequestIdentity = {
  key: string;
  generation: number;
};

export function isCurrentRequest(
  activeRequest: SessionRequestIdentity,
  capturedRequest: SessionRequestIdentity,
): boolean {
  return (
    activeRequest.key === capturedRequest.key &&
    activeRequest.generation === capturedRequest.generation
  );
}

/** Wrap state setters so callbacks from an obsolete request cannot mutate state. */
export function buildGuardedSetters(
  activeRequestRef: RefObject<SessionRequestIdentity>,
  capturedRequest: SessionRequestIdentity,
  setters: ResumeStateSetter,
): ResumeStateSetter {
  const guard = () => isCurrentRequest(activeRequestRef.current, capturedRequest);
  return {
    ...setters,
    setResumptionState: (s) => {
      if (guard()) setters.setResumptionState(s);
    },
    setError: (e) => {
      if (guard()) setters.setError(e);
    },
    setNotice: (notice) => {
      if (guard()) setters.setNotice?.(notice);
    },
    setWorktreePath: (p) => {
      if (guard()) setters.setWorktreePath(p);
    },
    setWorktreeBranch: (b) => {
      if (guard()) setters.setWorktreeBranch(b);
    },
    // Store setters are keyed by session id, but a stale callback can still
    // mutate that row after navigation unless every setter is guarded.
    setTaskSession: (s) => {
      if (guard()) setters.setTaskSession(s);
    },
    setAgentctlReady: (sid) => {
      if (guard()) setters.setAgentctlReady?.(sid);
    },
    setResumeSkipped: (sid, skipped) => {
      if (guard()) setters.setResumeSkipped?.(sid, skipped);
    },
  };
}
