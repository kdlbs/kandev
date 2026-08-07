"use client";

import { IconSparkles } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { GridSpinner } from "@/components/grid-spinner";
import { useTooltipMountGate } from "@/hooks/use-tooltip-mount-gate";
import { useTranslation } from "react-i18next";

type EnhancePromptButtonProps = {
  onClick: () => void;
  isLoading: boolean;
  isConfigured?: boolean;
};

export function EnhancePromptButton({
  onClick,
  isLoading,
  isConfigured = true,
}: EnhancePromptButtonProps) {
  const { t } = useTranslation();
  const { tooltipOpenState, handleTooltipOpenChange } = useTooltipMountGate();
  const isDisabled = !isConfigured || isLoading;
  const tooltipText = isConfigured
    ? t("common:enhancePromptWithAi")
    : t("common:configureAUtilityAgentToEnhance");

  return (
    <Tooltip open={tooltipOpenState} onOpenChange={handleTooltipOpenChange}>
      <TooltipTrigger asChild>
        {/* Wrap in span so tooltip works even when button is disabled */}
        <span
          className="inline-flex"
          tabIndex={isDisabled ? 0 : -1}
          aria-label={isDisabled ? tooltipText : undefined}
        >
          <span aria-hidden={isDisabled ? "true" : undefined} className="inline-flex">
            <Button
              type="button"
              variant="ghost"
              size="icon"
              className="h-7 w-7 cursor-pointer hover:bg-muted/40 text-slate-400"
              onClick={isConfigured ? onClick : undefined}
              disabled={isDisabled}
              aria-label={t("common:enhancePromptWithAi")}
              aria-busy={isLoading}
              data-testid="enhance-prompt-button"
            >
              {isLoading ? (
                <GridSpinner className="h-4 w-4" />
              ) : (
                <IconSparkles className="h-4 w-4" />
              )}
            </Button>
          </span>
        </span>
      </TooltipTrigger>
      <TooltipContent>{tooltipText}</TooltipContent>
    </Tooltip>
  );
}
