"use client";

import { useTranslation } from "react-i18next";
import type { PromptFormState } from "@/lib/settings/prompt-versions";

type PromptOrphanedDraftsNoticeProps = {
  drafts: { id: string; form: PromptFormState }[];
};

// PromptOrphanedDraftsNotice names prompts that were removed (for example, by
// Prompt Sync) while they had unsaved edits, and says how to keep or drop the
// text: Save recreates each as a new prompt, Reset discards it.
export function PromptOrphanedDraftsNotice({ drafts }: PromptOrphanedDraftsNoticeProps) {
  const { t } = useTranslation();
  if (drafts.length === 0) return null;
  const names = drafts.map((draft) => `@${draft.form.name}`).join(", ");
  return (
    <div
      role="status"
      className="rounded-lg border border-amber-500/60 bg-amber-500/10 p-4 text-sm text-foreground"
      data-testid="prompt-orphaned-drafts-notice"
    >
      {t("settings:promptRemovedWhileEditing", { count: drafts.length, names })}
    </div>
  );
}
