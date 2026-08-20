"use client";

import { IconChevronDown, IconCheck, IconX } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Spinner } from "@kandev/ui/spinner";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { KeyboardShortcutTooltip } from "@/components/keyboard-shortcut-tooltip";
import { SHORTCUTS } from "@/lib/keyboard/constants";
import { cn } from "@/lib/utils";
import { useTranslation } from "react-i18next";

type ClarificationHeaderActionsProps = {
  total: number;
  allAnswered: boolean;
  isSubmitting: boolean;
  onSubmit: () => void;
  onSkip: () => void;
  onCollapse?: () => void;
  collapseContentId?: string;
};

function ClarificationSkipButton({
  isSubmitting,
  onSkip,
  label,
}: {
  isSubmitting: boolean;
  onSkip: () => void;
  label: string;
}) {
  const button = (
    <span className="inline-flex" data-testid="clarification-skip-shortcut">
      <button
        type="button"
        onClick={onSkip}
        disabled={isSubmitting}
        className="text-muted-foreground hover:text-foreground cursor-pointer disabled:opacity-50"
        data-testid="clarification-skip"
        aria-label={label}
      >
        <IconX className="h-4 w-4" />
      </button>
    </span>
  );

  if (isSubmitting) return button;

  return (
    <Tooltip>
      <TooltipTrigger asChild>{button}</TooltipTrigger>
      <TooltipContent>{label}</TooltipContent>
    </Tooltip>
  );
}

export function ClarificationHeaderActions({
  total,
  allAnswered,
  isSubmitting,
  onSubmit,
  onSkip,
  onCollapse,
  collapseContentId,
}: ClarificationHeaderActionsProps) {
  const { t } = useTranslation();
  return (
    <div className="flex shrink-0 items-center gap-2">
      {total > 1 && (
        <KeyboardShortcutTooltip
          shortcut={SHORTCUTS.SUBMIT}
          description={t("task:submitAnswers")}
          enabled={!isSubmitting}
        >
          <span
            className="inline-flex"
            data-testid="clarification-submit-shortcut"
            tabIndex={!allAnswered && !isSubmitting ? 0 : undefined}
          >
            <button
              type="button"
              onClick={onSubmit}
              disabled={!allAnswered || isSubmitting}
              data-testid="clarification-submit"
              className={cn(
                "inline-flex items-center gap-1 text-xs px-3 py-1 rounded font-medium transition-colors",
                allAnswered && !isSubmitting
                  ? "bg-blue-500 text-white hover:bg-blue-500/90 cursor-pointer"
                  : "bg-muted text-muted-foreground cursor-not-allowed",
              )}
            >
              {isSubmitting ? t("task:submitting") : t("task:submit")}
              {isSubmitting ? (
                <Spinner aria-hidden="true" className="size-3" />
              ) : (
                <IconCheck className="h-3 w-3" />
              )}
            </button>
          </span>
        </KeyboardShortcutTooltip>
      )}
      <ClarificationSkipButton
        isSubmitting={isSubmitting}
        onSkip={onSkip}
        label={t("task:skipAllQuestions")}
      />
      {onCollapse && (
        <Button
          type="button"
          variant="ghost"
          size="icon"
          aria-label={t("chat:collapseClarification")}
          aria-expanded={true}
          aria-controls={collapseContentId}
          title={t("chat:collapseClarification")}
          data-testid="clarification-collapse-toggle"
          onClick={onCollapse}
          className="h-11 w-11 flex-shrink-0 cursor-pointer rounded-none"
        >
          <IconChevronDown className="h-4 w-4" />
        </Button>
      )}
    </div>
  );
}
