"use client";

import { IconSparkles } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { GridSpinner } from "@/components/grid-spinner";
import { useTooltipMountGate } from "@/hooks/use-tooltip-mount-gate";

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
  const { tooltipOpenState, handleTooltipOpenChange } = useTooltipMountGate();
  const isDisabled = !isConfigured || isLoading;
  const tooltipText = isConfigured
    ? "Enhance prompt with AI"
    : "Configure a utility agent in settings to enable AI enhancement";

  return (
    <Tooltip open={tooltipOpenState} onOpenChange={handleTooltipOpenChange}>
      <TooltipTrigger asChild>
        {/* Wrap in span so tooltip works even when button is disabled */}
        <span
          className="inline-flex"
          tabIndex={isDisabled ? 0 : -1}
          aria-label={isDisabled ? tooltipText : undefined}
        >
          <Button
            type="button"
            variant="ghost"
            size="icon"
            className="h-7 w-7 cursor-pointer hover:bg-muted/40 text-slate-400"
            onClick={isConfigured ? onClick : undefined}
            disabled={isDisabled}
            aria-label="Enhance prompt with AI"
            aria-busy={isLoading}
            data-testid="enhance-prompt-button"
          >
            {isLoading ? <GridSpinner className="h-4 w-4" /> : <IconSparkles className="h-4 w-4" />}
          </Button>
        </span>
      </TooltipTrigger>
      <TooltipContent>{tooltipText}</TooltipContent>
    </Tooltip>
  );
}
