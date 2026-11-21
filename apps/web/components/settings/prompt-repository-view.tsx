"use client";

import { useTranslation } from "react-i18next";
import { SettingsPromptEditor } from "@/components/settings/settings-prompt-editor";
import { repositoryVersion } from "@/lib/settings/prompt-versions";
import type { CustomPrompt } from "@/lib/types/http";

// PromptRepositoryView shows a synced prompt's or an override's repository
// version read-only, with the repository file where it is edited.
export function PromptRepositoryView({ prompt }: { prompt: CustomPrompt }) {
  const { t } = useTranslation();
  return (
    <div className="space-y-2">
      <SettingsPromptEditor
        value={repositoryVersion(prompt)}
        onChange={() => undefined}
        readOnly
        testId="prompt-view-content"
      />
      <p className="text-xs text-muted-foreground" data-testid="prompt-view-source-note">
        {t("settings:promptSyncedReadOnlyNote", { path: prompt.source_path ?? "" })}
      </p>
    </div>
  );
}
