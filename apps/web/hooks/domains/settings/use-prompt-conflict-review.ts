"use client";

import { useCallback, useMemo, useState } from "react";
import type {
  PromptConflictChoice,
  PromptConflictResolutions,
  PromptSyncConflict,
} from "@/lib/types/prompt-sync";

type ReviewState = { conflicts: PromptSyncConflict[]; choices: PromptConflictResolutions };

const EMPTY: ReviewState = { conflicts: [], choices: {} };

// usePromptConflictReview holds the prompts under review and the user's
// choice for each. Every prompt starts on the version it uses now when known,
// else on the given default ("local", the choice that changes no content for
// a conflict); reopening keeps choices already made for the same prompts.
export function usePromptConflictReview() {
  const [state, setState] = useState<ReviewState>(EMPTY);
  const open = useCallback(
    (conflicts: PromptSyncConflict[], defaultChoice: PromptConflictChoice = "local") =>
      setState((current) => ({
        conflicts,
        choices: Object.fromEntries(
          conflicts.map((conflict) => [
            conflict.name,
            (Object.hasOwn(current.choices, conflict.name)
              ? current.choices[conflict.name]
              : undefined) ??
              conflict.in_use ??
              defaultChoice,
          ]),
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
  return useMemo(
    () => ({
      active: state.conflicts.length > 0,
      conflicts: state.conflicts,
      choices: state.choices,
      open,
      setChoice,
      close,
    }),
    [state, open, setChoice, close],
  );
}

export type PromptConflictReview = ReturnType<typeof usePromptConflictReview>;
