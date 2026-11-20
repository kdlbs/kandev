"use client";

import { useMemo } from "react";
import { useCustomPrompts } from "@/hooks/domains/settings/use-custom-prompts";
import type { CustomPrompt } from "@/lib/types/http";

// isConflictedPrompt reports whether a prompt waits for the user to choose
// between its local and repository versions.
export function isConflictedPrompt(prompt: CustomPrompt): boolean {
  return prompt.source === "override" && prompt.conflict === true;
}

// usePromptConflictCount counts prompts waiting for the user to choose between
// the local and repository versions. It loads the prompt list if needed;
// prompt change events keep it current.
export function usePromptConflictCount(): number {
  const { prompts } = useCustomPrompts();
  return useMemo(() => prompts.filter(isConflictedPrompt).length, [prompts]);
}
