import {
  getStoredDismissedAgentErrors,
  setStoredDismissedAgentErrors,
} from "@/lib/session-last-agent-error";
import type { UISlice } from "./types";

type ImmerSet = (recipe: (draft: UISlice) => void) => void;

export function buildDismissedAgentErrors(set: ImmerSet, get: () => UISlice) {
  return {
    dismissedAgentErrors: getStoredDismissedAgentErrors(),
    dismissAgentError: (sessionId: string, stamp: string) => {
      if (!sessionId || !stamp) return;
      set((draft) => {
        draft.dismissedAgentErrors[sessionId] = stamp;
      });
      setStoredDismissedAgentErrors(get().dismissedAgentErrors);
    },
  };
}
