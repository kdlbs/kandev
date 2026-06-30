import {
  getStoredAcknowledgedAgentErrors,
  getStoredDismissedAgentErrors,
  setStoredAcknowledgedAgentErrors,
  setStoredDismissedAgentErrors,
} from "@/lib/session-last-agent-error";
import type { UISlice } from "./types";

type ImmerSet = (recipe: (draft: UISlice) => void) => void;

export function buildDismissedAgentErrors(set: ImmerSet) {
  return {
    acknowledgedAgentErrors: getStoredAcknowledgedAgentErrors(),
    dismissedAgentErrors: getStoredDismissedAgentErrors(),
    acknowledgeAgentError: (sessionId: string, stamp: string) => {
      if (!sessionId || !stamp) return;
      set((draft) => {
        draft.acknowledgedAgentErrors[sessionId] = stamp;
      });
      setStoredAcknowledgedAgentErrors({ [sessionId]: stamp });
    },
    dismissAgentError: (sessionId: string, stamp: string) => {
      if (!sessionId || !stamp) return;
      set((draft) => {
        draft.dismissedAgentErrors[sessionId] = stamp;
      });
      // Persist only the new entry. setStoredDismissedAgentErrors merges into
      // the current localStorage snapshot, so a concurrent tab's dismissals
      // for OTHER sessions survive, and only this single (sessionId, stamp)
      // pair can race with another tab — which is the intended scope.
      setStoredDismissedAgentErrors({ [sessionId]: stamp });
    },
  };
}
