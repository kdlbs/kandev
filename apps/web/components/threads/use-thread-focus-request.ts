import { useState } from "react";

/** Keeps visual dismissal separate from the initial conversation's mount lifetime. */
export function useThreadFocusRequest(
  focusedTaskId: string | null,
  focusRequestKey: string | null,
) {
  const [retired, setRetired] = useState(false);
  const [requested, setRequested] = useState(focusRequestKey);
  const [activationTaskId, setActivationTaskId] = useState(focusedTaskId);
  if (requested !== focusRequestKey) {
    setRequested(focusRequestKey);
    setRetired(false);
    setActivationTaskId(focusedTaskId);
  } else if ((!retired || !focusedTaskId) && activationTaskId !== focusedTaskId) {
    // A consumed request loses its fallback only when its target leaves the deck.
    setActivationTaskId(focusedTaskId);
  }
  return {
    markedTaskId: retired ? null : focusedTaskId,
    activationTaskId,
    retire: () => setRetired(true),
  };
}
