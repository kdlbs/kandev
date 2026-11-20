"use client";

import { useTranslation } from "react-i18next";
import { SettingsPromptEditor } from "@/components/settings/settings-prompt-editor";
import { localVersion, repositoryVersion } from "@/lib/settings/prompt-versions";
import type { CustomPrompt } from "@/lib/types/http";
import type { PromptConflictChoice } from "@/lib/types/prompt-sync";

type PromptVersionPreviewProps = {
  prompt: CustomPrompt;
  choice: PromptConflictChoice;
  /** Test ID of the read-only editor; the edit form reuses its editor's ID. */
  testId?: string;
};

// PromptVersionPreview shows, read-only, the version a synced prompt or an
// override uses with the given choice: the repository version with the file
// where it is edited, or the local version with a pointer to Edit.
export function PromptVersionPreview({
  prompt,
  choice,
  testId = "prompt-version-preview",
}: PromptVersionPreviewProps) {
  const { t } = useTranslation();
  const local = choice === "local";
  return (
    <div className="space-y-2">
      <SettingsPromptEditor
        value={local ? localVersion(prompt) : repositoryVersion(prompt)}
        onChange={() => undefined}
        readOnly
        testId={testId}
      />
      <p className="text-xs text-muted-foreground" data-testid="prompt-view-source-note">
        {local
          ? t("settings:promptVersionPreviewLocalNote")
          : t("settings:promptSyncedReadOnlyNote", { path: prompt.source_path ?? "" })}
      </p>
    </div>
  );
}
