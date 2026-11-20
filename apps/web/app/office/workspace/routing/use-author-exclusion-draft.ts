"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import type { AuthorExclusionStep, AuthorExclusionStepChange } from "@/lib/types/author-exclusion";
import { authorExclusionStepKey, type AuthorExclusionDraft } from "./components";

/**
 * Draft state for the Review on a different provider switches. The draft
 * holds only touched switches and resets whenever the saved steps change, so
 * a successful save or reload starts clean.
 */
export function useAuthorExclusionDraft(steps: AuthorExclusionStep[]) {
  const [draft, setDraft] = useState<AuthorExclusionDraft>({});
  const savedSignature = steps
    .map((step) => `${authorExclusionStepKey(step)}=${step.enabled}`)
    .join("|");

  useEffect(() => {
    setDraft({});
  }, [savedSignature]);

  const setValue = useCallback((key: string, enabled: boolean) => {
    setDraft((current) => ({ ...current, [key]: enabled }));
  }, []);

  const changes = useMemo<AuthorExclusionStepChange[]>(
    () =>
      steps.flatMap((step) => {
        const value = draft[authorExclusionStepKey(step)];
        if (value === undefined || value === step.enabled) return [];
        return [{ step_id: step.step_id, role: step.role, enabled: value }];
      }),
    [draft, steps],
  );

  const reset = useCallback(() => setDraft({}), []);

  return { draft, setValue, changes, reset };
}
