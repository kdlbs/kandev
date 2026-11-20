"use client";

import { useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { Badge } from "@kandev/ui/badge";
import { Label } from "@kandev/ui/label";
import { Switch } from "@kandev/ui/switch";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { InlineConfirmActions } from "@/components/confirmation/inline-confirm-actions";
import { MobileActionConfirmation } from "@/components/confirmation/mobile-action-confirmation";
import { usePromptOverride } from "@/hooks/domains/settings/use-prompt-override";
import type { CustomPrompt } from "@/lib/types/http";

// PromptOverrideToggle is the labelled "Local override" switch on synced and
// overridden prompts. Turning it on makes a local, editable copy; turning it
// off discards that copy, so it asks for confirmation first.
export function PromptOverrideToggle({ prompt }: { prompt: CustomPrompt }) {
  const { t } = useTranslation();
  const { pendingId, override, revert } = usePromptOverride();
  const [confirming, setConfirming] = useState(false);
  const anchorRef = useRef<HTMLButtonElement>(null);
  const overridden = prompt.source === "override";
  const switchId = `prompt-override-${prompt.id}`;
  const onCheckedChange = (checked: boolean) => {
    if (checked) void override(prompt.id);
    else setConfirming(true);
  };
  const confirmRevert = async () => {
    setConfirming(false);
    await revert(prompt.id);
  };
  const cancel = () => setConfirming(false);
  const description = t("settings:promptRevertConfirm");
  const confirmLabel = t("settings:promptRevertConfirmAction");

  return (
    <div className="flex flex-wrap items-center justify-end gap-2">
      <Label
        htmlFor={switchId}
        className="flex cursor-pointer items-center gap-2 text-xs font-normal text-muted-foreground max-md:min-h-11 [@media(pointer:coarse)]:min-h-11"
        data-testid="prompt-override-switch-label"
      >
        <Switch
          ref={anchorRef}
          id={switchId}
          checked={overridden}
          onCheckedChange={onCheckedChange}
          disabled={pendingId === prompt.id || confirming}
          className="cursor-pointer"
          data-testid="prompt-override-switch"
        />
        {t("settings:promptLocalOverride")}
      </Label>
      <MobileActionConfirmation
        open={confirming}
        targetKey={prompt.id}
        title={t("settings:promptRevertOverride")}
        subject={`@${prompt.name}`}
        description={description}
        cancelLabel={t("common:cancel")}
        confirmLabel={confirmLabel}
        confirmAriaLabel={confirmLabel}
        confirmTestId="prompt-revert-confirm"
        testId="prompt-revert-confirmation"
        onOpenChange={(open) => {
          if (!open) cancel();
        }}
        onConfirm={confirmRevert}
        focusReturnRef={anchorRef}
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
            onCancel={cancel}
            onConfirm={confirmRevert}
          />
        }
      />
    </div>
  );
}

// PromptOverrideBadge marks a local override and names the repository file
// it replaces.
export function PromptOverrideBadge({ prompt }: { prompt: CustomPrompt }) {
  const { t } = useTranslation();
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Badge
          variant="outline"
          tabIndex={0}
          className="text-xs cursor-default"
          data-testid="prompt-override-badge"
        >
          {t("settings:promptOverridden")}
        </Badge>
      </TooltipTrigger>
      <TooltipContent>
        {t("settings:promptOverriddenTooltip", { path: prompt.source_path ?? "" })}
      </TooltipContent>
    </Tooltip>
  );
}
