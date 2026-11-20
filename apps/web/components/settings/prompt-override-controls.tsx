"use client";

import { useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { IconLock } from "@tabler/icons-react";
import { Badge } from "@kandev/ui/badge";
import { Label } from "@kandev/ui/label";
import { Switch } from "@kandev/ui/switch";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { InlineConfirmActions } from "@/components/confirmation/inline-confirm-actions";
import { MobileActionConfirmation } from "@/components/confirmation/mobile-action-confirmation";
import { usePromptOverride } from "@/hooks/domains/settings/use-prompt-override";
import type { CustomPrompt } from "@/lib/types/http";

// usePromptOverrideSwitch owns one row's "Local override" switch state.
// Turning the switch on overrides immediately; turning it off waits for the
// confirmation, which the row renders below its header so the switch stays
// in place.
export function usePromptOverrideSwitch(prompt: CustomPrompt) {
  const { pendingId, override, revert } = usePromptOverride();
  const [confirming, setConfirming] = useState(false);
  const anchorRef = useRef<HTMLButtonElement>(null);
  return {
    prompt,
    anchorRef,
    confirming,
    pending: pendingId === prompt.id,
    onCheckedChange: (checked: boolean) => {
      if (checked) void override(prompt.id);
      else setConfirming(true);
    },
    confirm: async () => {
      setConfirming(false);
      await revert(prompt.id);
    },
    cancel: () => setConfirming(false),
  };
}

export type PromptOverrideSwitchState = ReturnType<typeof usePromptOverrideSwitch>;

// PromptOverrideSwitch is the labelled "Local override" switch, the first of
// a synced or overridden row's trailing controls.
export function PromptOverrideSwitch({ state }: { state: PromptOverrideSwitchState }) {
  const { t } = useTranslation();
  const switchId = `prompt-override-${state.prompt.id}`;
  return (
    <Label
      htmlFor={switchId}
      className="flex shrink-0 cursor-pointer items-center gap-2 px-1 text-xs font-normal text-muted-foreground max-md:min-h-11 [@media(pointer:coarse)]:min-h-11"
      data-testid="prompt-override-switch-label"
    >
      <Switch
        ref={state.anchorRef}
        id={switchId}
        checked={state.prompt.source === "override"}
        onCheckedChange={state.onCheckedChange}
        disabled={state.pending || state.confirming}
        className="cursor-pointer"
        data-testid="prompt-override-switch"
      />
      {t("settings:promptLocalOverride")}
    </Label>
  );
}

// PromptRevertConfirmation asks before the repository version replaces the
// local one: inline below the row header on desktop, a sheet on phones.
export function PromptRevertConfirmation({ state }: { state: PromptOverrideSwitchState }) {
  const { t } = useTranslation();
  const description = t("settings:promptRevertConfirm");
  const confirmLabel = t("settings:promptRevertConfirmAction");
  return (
    <MobileActionConfirmation
      open={state.confirming}
      targetKey={state.prompt.id}
      title={t("settings:promptRevertOverride")}
      subject={`@${state.prompt.name}`}
      description={description}
      cancelLabel={t("common:cancel")}
      confirmLabel={confirmLabel}
      confirmAriaLabel={confirmLabel}
      confirmTestId="prompt-revert-confirm"
      testId="prompt-revert-confirmation"
      onOpenChange={(open) => {
        if (!open) state.cancel();
      }}
      onConfirm={state.confirm}
      focusReturnRef={state.anchorRef}
      fallback={
        <InlineConfirmActions
          density="touch"
          testId="prompt-revert-confirmation"
          ariaLabel={t("settings:promptRevertOverride")}
          description={description}
          cancelLabel={t("common:cancel")}
          confirmLabel={confirmLabel}
          confirmAriaLabel={confirmLabel}
          confirmTestId="prompt-revert-confirm"
          onCancel={state.cancel}
          onConfirm={state.confirm}
        />
      }
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
export function PromptOverrideBadge({ prompt }: { prompt: CustomPrompt }) {
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
export function PromptConflictBadge({ prompt }: { prompt: CustomPrompt }) {
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
      {overridden && prompt.conflict && <PromptConflictBadge prompt={prompt} />}
    </>
  );
}
