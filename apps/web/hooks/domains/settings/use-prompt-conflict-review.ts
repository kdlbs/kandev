"use client";

import { useCallback, useState } from "react";
import type {
  PromptConflictChoice,
  PromptConflictResolutions,
  PromptSyncConflict,
} from "@/lib/types/prompt-sync";

type ReviewState = { conflicts: PromptSyncConflict[]; choices: PromptConflictResolutions };

const EMPTY: ReviewState = { conflicts: [], choices: {} };

// usePromptConflictReview holds the conflicts under review and the user's
// choice for each. Every conflict starts on "local", the choice that changes
// no content; reopening keeps choices already made for the same prompts.
export function usePromptConflictReview() {
  const [state, setState] = useState<ReviewState>(EMPTY);
  const open = useCallback(
    (conflicts: PromptSyncConflict[]) =>
      setState((current) => ({
        conflicts,
        choices: Object.fromEntries(
          conflicts.map((conflict) => [conflict.name, current.choices[conflict.name] ?? "local"]),
        ),
      })),
    [],
  );
  const setChoice = useCallback(
    (name: string, choice: PromptConflictChoice) =>
      setState((current) => ({ ...current, choices: { ...current.choices, [name]: choice } })),
    [],
  );
  const close = useCallback(() => setState(EMPTY), []);
  return {
    active: state.conflicts.length > 0,
    conflicts: state.conflicts,
    choices: state.choices,
    open,
    setChoice,
    close,
  };
}

export type PromptConflictReview = ReturnType<typeof usePromptConflictReview>;
