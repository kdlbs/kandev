"use client";

import { IconEye, IconEyeOff } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { useTranslation } from "react-i18next";

type TaskCreateLaunchPreviewToggleProps = {
  active: boolean;
  disabled?: boolean;
  stepName: string;
  onToggle: () => void;
};

export function TaskCreateLaunchPreviewToggle({
  active,
  disabled,
  stepName,
  onToggle,
}: TaskCreateLaunchPreviewToggleProps) {
  const { t } = useTranslation();
  const label = active
    ? t("task:editTaskPrompt")
    : t("task:previewLaunchPrompt", { step: stepName });
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <span tabIndex={disabled ? 0 : -1} className="inline-flex">
          <Button
            type="button"
            variant="ghost"
            size="icon"
            className="h-7 w-7 cursor-pointer text-slate-400 hover:bg-muted/40 [@media(pointer:coarse)]:h-11 [@media(pointer:coarse)]:w-11"
            aria-label={label}
            aria-pressed={active}
            data-testid="task-create-launch-preview-toggle"
            disabled={disabled}
            onClick={onToggle}
          >
            {active ? <IconEyeOff /> : <IconEye />}
          </Button>
        </span>
      </TooltipTrigger>
      <TooltipContent>{label}</TooltipContent>
    </Tooltip>
  );
}

export function TaskCreateLaunchPreviewContent({ content }: { content: string }) {
  const { t } = useTranslation();
  return (
    <div
      role="textbox"
      aria-readonly="true"
      aria-label={t("task:launchPromptPreview")}
      data-testid="task-create-launch-preview-content"
      className="min-h-[96px] max-h-[240px] min-w-0 max-w-full overflow-auto whitespace-pre-wrap break-words [overflow-wrap:anywhere] px-3 py-2 text-[13px]"
    >
      {content}
    </div>
  );
}
