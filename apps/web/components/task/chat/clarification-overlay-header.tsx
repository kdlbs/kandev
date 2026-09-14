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
        className="inline-flex h-6 w-6 cursor-pointer items-center justify-center rounded text-muted-foreground transition-[color,background-color,transform] duration-150 hover:bg-muted hover:text-foreground active:scale-[0.96] disabled:opacity-50 [@media(pointer:coarse)]:h-11 [@media(pointer:coarse)]:w-11 [@media(pointer:coarse)]:rounded-lg [@media(pointer:coarse)]:bg-muted/60"
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
    <div
      className={cn(
        "flex shrink-0 items-center justify-end gap-2",
        total > 1 && "w-full md:w-auto",
      )}
    >
      {total > 1 && (
        <KeyboardShortcutTooltip
          shortcut={SHORTCUTS.SUBMIT}
          description={t("task:submitAnswers")}
          enabled={!isSubmitting}
        >
          <span
            className="inline-flex min-w-0 flex-1 md:flex-none"
            data-testid="clarification-submit-shortcut"
            tabIndex={!allAnswered && !isSubmitting ? 0 : undefined}
          >
            <button
              type="button"
              onClick={onSubmit}
              disabled={!allAnswered || isSubmitting}
              aria-label={t("task:submit")}
              data-testid="clarification-submit"
              className={cn(
                "inline-flex w-full items-center justify-center gap-2 rounded-lg px-4 py-1 text-xs font-medium transition-[color,background-color,transform] duration-150 active:scale-[0.96] md:w-auto md:gap-1 md:rounded md:px-3 [@media(pointer:coarse)]:min-h-11",
                allAnswered && !isSubmitting
                  ? "cursor-pointer bg-blue-500 text-white shadow-sm hover:bg-blue-500/90"
                  : "bg-muted text-muted-foreground cursor-not-allowed",
              )}
            >
              {isSubmitting ? t("task:submitting") : t("task:submit")}
              {!isSubmitting && <IconCheck className="h-3 w-3" />}
            </button>
          </span>
        </KeyboardShortcutTooltip>
      )}
      {isSubmitting && (
        <Spinner
          aria-label={t("task:submitting")}
          data-testid="clarification-submitting-status"
          className="size-3 shrink-0"
        />
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
          className="h-6 w-6 flex-shrink-0 cursor-pointer rounded transition-[color,background-color,transform] duration-150 active:scale-[0.96] [@media(pointer:coarse)]:h-11 [@media(pointer:coarse)]:w-11 [@media(pointer:coarse)]:rounded-lg [@media(pointer:coarse)]:bg-muted/60"
        >
          <IconChevronDown className="h-4 w-4" />
        </Button>
      )}
    </div>
  );
}
