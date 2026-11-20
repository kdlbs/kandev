"use client";

import { useMemo } from "react";
import { useCustomPrompts } from "@/hooks/domains/settings/use-custom-prompts";

// usePromptConflictCount counts prompts waiting for the user to choose between
// the local and repository versions. It loads the prompt list if needed;
// prompt change events keep it current.
export function usePromptConflictCount(): number {
  const { prompts } = useCustomPrompts();
  return useMemo(
    () => prompts.filter((prompt) => prompt.source === "override" && prompt.conflict).length,
    [prompts],
  );
}
