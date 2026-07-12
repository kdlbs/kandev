"use client";

import { useEffect, useRef } from "react";
import type { Repository } from "@/lib/types/http";
import type { DialogFormState } from "@/components/task-create-dialog-types";
import { resolveStartupPromptForManualDialog } from "@/lib/repository/startup-prompt";

/**
 * Watches the first selected repository + task title and pre-fills the
 * description input with the repository's startup_prompt, resolved for the
 * manual dialog (TASK_TITLE substitution + dropped ticket lines).
 *
 * Rules (see spec):
 *  - Only fires in create mode. Edit mode opens an existing task whose
 *    description belongs to the user; injecting a repository default into
 *    an empty description there would silently rewrite persisted state on
 *    save. Session mode is likewise a no-op — the mode targets an existing
 *    session, not a new task.
 *  - Only fires when the description input is empty OR still holds the last
 *    prompt this effect wrote — never clobbers user text.
 *  - Re-runs when the selected repo changes, so switching repos re-pre-fills
 *    with the new repo's prompt (subject to the "not user-edited" rule).
 */
export function useRepositoryStartupPromptPrefillEffect(
  fs: DialogFormState,
  open: boolean,
  repositories: Repository[],
  taskName: string,
  isCreateMode: boolean,
): void {
  const { descriptionInputRef, setHasDescription, repositories: rows } = fs;
  const lastAppliedRef = useRef<string>("");

  // Scan for the first row with an actual selection — rows[0] can be an
  // empty placeholder (autopick leaves one when no stored preference matches,
  // useRepositoriesState initialises new rows without one).
  const firstRepoId = rows.find((r) => r.repositoryId)?.repositoryId ?? "";
  const startupPrompt = firstRepoId
    ? (repositories.find((r) => r.id === firstRepoId)?.startup_prompt ?? "")
    : "";

  useEffect(() => {
    if (!open || !isCreateMode) {
      lastAppliedRef.current = "";
      return;
    }
    const currentValue = descriptionInputRef.current?.getValue() ?? "";
    // Only compare against the last prompt we wrote. Treating "" as untouched
    // re-fills the prompt after the user has intentionally cleared it — every
    // taskName keystroke would then undo the clear. lastAppliedRef starts as
    // "", so the initial-open case where the input is still empty still
    // triggers the first fill.
    if (currentValue !== lastAppliedRef.current) {
      return;
    }
    const resolved = resolveStartupPromptForManualDialog(startupPrompt, taskName);
    if (resolved === currentValue) {
      return;
    }
    descriptionInputRef.current?.setValue(resolved);
    lastAppliedRef.current = resolved;
    setHasDescription(resolved.length > 0);
  }, [open, isCreateMode, startupPrompt, taskName, descriptionInputRef, setHasDescription]);
}
