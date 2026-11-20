"use client";

import { useTranslation } from "react-i18next";
import { IconArrowBackUp, IconPencilPlus } from "@tabler/icons-react";
import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { usePromptOverride } from "@/hooks/domains/settings/use-prompt-override";
import type { CustomPrompt } from "@/lib/types/http";

const touchSizeClass = "cursor-pointer max-md:size-11 [@media(pointer:coarse)]:size-11";

// PromptOverrideButton turns a synced prompt into a local, editable override.
export function PromptOverrideButton({ prompt }: { prompt: CustomPrompt }) {
  const { t } = useTranslation();
  const { pendingId, override } = usePromptOverride();
  return (
    <Button
      variant="ghost"
      size="icon"
      onClick={() => void override(prompt.id)}
      disabled={pendingId === prompt.id}
      aria-label={t("settings:promptOverride")}
      title={t("settings:promptOverride")}
      className={touchSizeClass}
      data-testid="prompt-override-button"
    >
      <IconPencilPlus className="h-4 w-4" />
    </Button>
  );
}

// PromptOverrideControls marks a local override of a synced prompt and offers
// Revert, which returns it to the repository version.
export function PromptOverrideControls({ prompt }: { prompt: CustomPrompt }) {
  const { t } = useTranslation();
  const { pendingId, revert } = usePromptOverride();
  return (
    <>
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
      <Button
        variant="ghost"
        size="icon"
        onClick={() => void revert(prompt.id)}
        disabled={pendingId === prompt.id}
        aria-label={t("settings:promptRevertOverride")}
        title={t("settings:promptRevertOverride")}
        className={touchSizeClass}
        data-testid="prompt-revert-button"
      >
        <IconArrowBackUp className="h-4 w-4" />
      </Button>
    </>
  );
}
