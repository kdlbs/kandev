"use client";

import { useTranslation } from "react-i18next";
import { IconLock } from "@tabler/icons-react";
import { Badge } from "@kandev/ui/badge";
import { Label } from "@kandev/ui/label";
import { Switch } from "@kandev/ui/switch";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { usePromptOverride } from "@/hooks/domains/settings/use-prompt-override";
import { isConflictedPrompt } from "@/hooks/domains/settings/use-prompt-conflict-count";
import type { CustomPrompt } from "@/lib/types/http";

// PromptOverrideSwitch is the labelled "Local override" switch, the first of
// a synced or overridden row's trailing controls. Both directions apply at
// once: neither version is lost, and a toast names the version now in use.
export function PromptOverrideSwitch({ prompt }: { prompt: CustomPrompt }) {
  const { t } = useTranslation();
  const { pendingId, override, revert } = usePromptOverride();
  const switchId = `prompt-override-${prompt.id}`;
  return (
    <Label
      htmlFor={switchId}
      className="flex shrink-0 cursor-pointer items-center gap-2 px-1 text-xs font-normal text-muted-foreground max-md:min-h-11 [@media(pointer:coarse)]:min-h-11"
      data-testid="prompt-override-switch-label"
    >
      <Switch
        id={switchId}
        checked={prompt.source === "override"}
        onCheckedChange={(checked) => {
          void (checked ? override(prompt.id, prompt.name) : revert(prompt.id, prompt.name));
        }}
        disabled={pendingId === prompt.id}
        className="cursor-pointer"
        data-testid="prompt-override-switch"
      />
      {t("settings:promptLocalOverride")}
    </Label>
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
      tooltip={t("settings:promptOverriddenTooltip", { path: prompt.source_path ?? "" })}
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
