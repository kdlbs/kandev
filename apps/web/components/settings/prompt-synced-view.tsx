"use client";

import { useState } from "react";
import { useTranslation } from "react-i18next";
import { IconEye, IconEyeOff } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { PromptOverrideButton } from "@/components/settings/prompt-override-controls";
import { PromptSyncedBadge } from "@/components/settings/prompt-synced-badge";
import { SettingsPromptEditor } from "@/components/settings/settings-prompt-editor";
import type { CustomPrompt } from "@/lib/types/http";

type PromptViewButtonProps = {
  promptId: string;
  expanded: boolean;
  onToggle: () => void;
};

// PromptViewButton toggles the read-only content of a synced prompt. Its
// accessible name stays stable; aria-expanded reports the state.
function PromptViewButton({ promptId, expanded, onToggle }: PromptViewButtonProps) {
  const { t } = useTranslation();
  const Icon = expanded ? IconEyeOff : IconEye;
  return (
    <Button
      variant="ghost"
      size="icon"
      onClick={onToggle}
      aria-label={t("settings:promptViewContent")}
      aria-expanded={expanded}
      aria-controls={`prompt-view-${promptId}`}
      className="cursor-pointer max-md:size-11 [@media(pointer:coarse)]:size-11"
      data-testid="prompt-view-button"
    >
      <Icon className="h-4 w-4" />
    </Button>
  );
}

// PromptSyncedContent shows a synced prompt's full content read-only, with a
// note naming the repository file where it is edited.
function PromptSyncedContent({ prompt }: { prompt: CustomPrompt }) {
  const { t } = useTranslation();
  return (
    <div className="space-y-2" id={`prompt-view-${prompt.id}`}>
      <SettingsPromptEditor
        value={prompt.content}
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

// SyncedPromptListItem renders a Prompt Sync-owned prompt: name, synced badge,
// and a View toggle in place of edit and delete, which the repository owns.
export function SyncedPromptListItem({ prompt }: { prompt: CustomPrompt }) {
  const [viewing, setViewing] = useState(false);
  return (
    <div
      className="rounded-lg border border-border/70 bg-background p-4 flex flex-col gap-3"
      data-testid="prompt-list-item"
      data-prompt-name={prompt.name}
    >
      <div className="flex items-center justify-between gap-2">
        <div className="flex items-center gap-2">
          <div className="text-sm font-medium text-foreground">@{prompt.name}</div>
          <PromptSyncedBadge sourcePath={prompt.source_path} />
        </div>
        <div className="flex items-center gap-1">
          <PromptOverrideButton prompt={prompt} />
          <PromptViewButton
            promptId={prompt.id}
            expanded={viewing}
            onToggle={() => setViewing((open) => !open)}
          />
        </div>
      </div>
      {viewing ? (
        <PromptSyncedContent prompt={prompt} />
      ) : (
        <div className="text-xs text-muted-foreground whitespace-pre-wrap">
          <div className="truncate">{prompt.content.split(/\r?\n/)[0] ?? ""}</div>
        </div>
      )}
    </div>
  );
}
