"use client";

import { IconChevronDown, IconCheck, IconX, IconMessageQuestion } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Spinner } from "@kandev/ui/spinner";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { KeyboardShortcutTooltip } from "@/components/keyboard-shortcut-tooltip";
import { SHORTCUTS } from "@/lib/keyboard/constants";
import { cn } from "@/lib/utils";
import { useTranslation } from "react-i18next";
import { ClarificationStepper } from "./clarification-overlay-parts";

function clarificationHeaderClassName(total: number): string {
  return cn(
    "flex min-h-11 justify-between",
    total > 1
      ? "flex-col items-stretch gap-2 px-3 py-2 md:flex-row md:items-center md:gap-3 md:px-4 md:py-0"
      : "items-center gap-3 px-4",
  );
}

type ClarificationOverlayTopBarProps = {
  total: number;
  activeIndex: number;
  isAnswered: (index: number) => boolean;
  onJump: (index: number) => void;
  isSubmitting: boolean;
  answeredCount: number;
  answerableTotal: number;
  allAnswered: boolean;
  onSubmit: () => void;
  onSkip: () => void;
  lateMode?: boolean;
  onLateClose?: () => void;
  onCollapse?: () => void;
  collapseContentId?: string;
};

// Groups the stepper, progress text, and header actions row shared by
// ClarificationInputOverlay -- kept here (rather than inline) so that file
// stays under the repo's max-lines-per-function/file limits.
export function ClarificationOverlayTopBar({
  total,
  activeIndex,
  isAnswered,
  onJump,
  isSubmitting,
  answeredCount,
  answerableTotal,
  allAnswered,
  onSubmit,
  onSkip,
  lateMode = false,
  onLateClose,
  onCollapse,
  collapseContentId,
}: ClarificationOverlayTopBarProps) {
  const { t } = useTranslation();
  return (
    <div className={clarificationHeaderClassName(total)} data-testid="clarification-overlay-header">
      <div className="flex min-w-0 flex-1 items-center gap-3">
        <IconMessageQuestion className="h-4 w-4 text-blue-500 flex-shrink-0" />
        {total > 1 && (
          <ClarificationStepper
            total={total}
            activeIndex={activeIndex}
            isAnswered={isAnswered}
            onJump={onJump}
            isSubmitting={isSubmitting}
          />
        )}
        {total > 1 && (
          <span
            data-testid="clarification-group-progress"
            className="ml-auto min-w-0 truncate text-xs text-muted-foreground md:ml-0"
          >
            {t("task:answeredOfTotal", { answeredCount, total: answerableTotal })}
          </span>
        )}
      </div>
      <ClarificationHeaderActions
        total={total}
        allAnswered={allAnswered}
        isSubmitting={isSubmitting}
        onSubmit={onSubmit}
        onSkip={onSkip}
        lateMode={lateMode}
        onLateClose={onLateClose}
        onCollapse={onCollapse}
        collapseContentId={collapseContentId}
      />
    </div>
  );
}

type ClarificationHeaderActionsProps = {
  total: number;
  allAnswered: boolean;
  isSubmitting: boolean;
  onSubmit: () => void;
  onSkip: () => void;
  lateMode?: boolean;
  onLateClose?: () => void;
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

// eslint-disable-next-line max-lines-per-function, complexity -- header actions keep active and late-answer controls aligned.
export function ClarificationHeaderActions({
  total,
  allAnswered,
  isSubmitting,
  onSubmit,
  onSkip,
  lateMode = false,
  onLateClose,
  onCollapse,
  collapseContentId,
}: ClarificationHeaderActionsProps) {
  const { t } = useTranslation();
  if (lateMode) {
    return (
      <div className="flex shrink-0 items-center justify-end gap-2">
        <Button
          type="button"
          variant="ghost"
          size="sm"
          className="min-h-11 cursor-pointer md:min-h-0"
          onClick={onLateClose}
          disabled={isSubmitting}
          data-testid="clarification-late-close"
        >
          {t("task:closeClarification")}
        </Button>
        <Button
          type="button"
          size="sm"
          className="min-h-11 cursor-pointer gap-1.5 md:min-h-0"
          onClick={onSubmit}
          disabled={!allAnswered || isSubmitting}
          data-testid="clarification-late-submit"
        >
          {isSubmitting ? t("task:lateAnswerSending") : t("task:lateAnswerSend")}
          {!isSubmitting && <IconCheck className="h-3 w-3" />}
        </Button>
      </div>
    );
  }
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
