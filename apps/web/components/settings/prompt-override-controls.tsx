"use client";

import { useTranslation } from "react-i18next";
import { IconLock } from "@tabler/icons-react";
import { Badge } from "@kandev/ui/badge";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { VersionSwitch } from "@/components/settings/prompt-conflict-review";
import { isConflictedPrompt } from "@/hooks/domains/settings/use-prompt-conflict-count";
import { hasRepositoryVersion } from "@/lib/settings/prompt-versions";
import type { PromptConflictChoice } from "@/lib/types/prompt-sync";
import type { CustomPrompt } from "@/lib/types/http";

type PromptVersionControlProps = {
  prompt: CustomPrompt;
  choice: PromptConflictChoice;
  disabled?: boolean;
  onChoose: (choice: PromptConflictChoice) => void;
};

// PromptVersionControl is a synced or overridden row's Local | Repository
// switch, the first of its trailing controls. Changing it is a pending edit
// that the page's save bar applies or resets. An override whose file left the
// repository has only its local version, so it shows that instead.
export function PromptVersionControl({
  prompt,
  choice,
  disabled,
  onChoose,
}: PromptVersionControlProps) {
  const { t } = useTranslation();
  if (!hasRepositoryVersion(prompt)) {
    return (
      <span className="text-xs text-muted-foreground" data-testid="prompt-repository-removed">
        {t("settings:promptRepositoryRemoved")}
      </span>
    );
  }
  return (
    <VersionSwitch
      name={prompt.name}
      choice={choice}
      onChoose={onChoose}
      disabled={disabled}
      idPrefix={`prompt-version-${prompt.id}`}
      switchTestId="prompt-override-switch"
      labelTestId="prompt-override-switch-label"
    />
  );
}

function SourceBadge({
  label,
  tooltip,
  testId,
  className,
}: {
  label: string;
  tooltip: string;
  testId: string;
  className?: string;
}) {
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Badge
          variant="outline"
          tabIndex={0}
          className={`text-xs cursor-default ${className ?? ""}`}
          data-testid={testId}
        >
          {label}
        </Badge>
      </TooltipTrigger>
      <TooltipContent>{tooltip}</TooltipContent>
    </Tooltip>
  );
}

// PromptOverrideBadge marks a local override and names the repository file
// it replaces.
function PromptOverrideBadge({ prompt }: { prompt: CustomPrompt }) {
  const { t } = useTranslation();
  return (
    <SourceBadge
      label={t("settings:promptOverridden")}
      tooltip={t(
        hasRepositoryVersion(prompt)
          ? "settings:promptOverriddenTooltip"
          : "settings:promptRepositoryRemovedTooltip",
        { path: prompt.source_path ?? "" },
      )}
      testId="prompt-override-badge"
    />
  );
}

// PromptConflictBadge marks an override whose name a repository file also
// uses until the user chooses a version.
function PromptConflictBadge({ prompt }: { prompt: CustomPrompt }) {
  const { t } = useTranslation();
  return (
    <SourceBadge
      label={t("settings:promptConflict")}
      tooltip={t("settings:promptConflictTooltip", { path: prompt.source_path ?? "" })}
      testId="prompt-conflict-badge"
      className="border-amber-500/60 text-amber-600 dark:text-amber-400"
    />
  );
}

// PromptRowBadges are the ownership badges after an editable row's name:
// built-in, overridden, and conflict.
export function PromptRowBadges({ prompt }: { prompt: CustomPrompt }) {
  const { t } = useTranslation();
  const overridden = prompt.source === "override";
  return (
    <>
      {prompt.builtin && (
        <Badge variant="secondary" className="text-xs">
          <IconLock className="h-3 w-3 mr-1" />
          {t("settings:builtIn")}
        </Badge>
      )}
      {overridden && <PromptOverrideBadge prompt={prompt} />}
      {isConflictedPrompt(prompt) && <PromptConflictBadge prompt={prompt} />}
    </>
  );
}
