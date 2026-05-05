"use client";

import { IconCheck, IconCornerDownLeft, IconArrowLeft, IconArrowRight } from "@tabler/icons-react";
import { cn } from "@/lib/utils";
import type { ClarificationOption } from "@/lib/types/http";

export function stepClassName(active: boolean, answered: boolean): string {
  if (active) {
    return "bg-blue-500 text-white border-blue-500 shadow-[0_0_0_3px_rgba(59,130,246,0.18)]";
  }
  if (answered) {
    return "bg-blue-500/20 text-blue-600 border-blue-500/40 dark:text-blue-300";
  }
  return "bg-muted text-muted-foreground border-border hover:bg-muted/70";
}

type StepperProps = {
  total: number;
  activeIndex: number;
  isAnswered: (index: number) => boolean;
  onJump: (index: number) => void;
  isSubmitting: boolean;
};

export function ClarificationStepper({
  total,
  activeIndex,
  isAnswered,
  onJump,
  isSubmitting,
}: StepperProps) {
  return (
    <div
      className="flex items-center gap-1.5 select-none"
      role="tablist"
      data-testid="clarification-stepper"
    >
      {Array.from({ length: total }).map((_, i) => {
        const answered = isAnswered(i);
        const active = i === activeIndex;
        return (
          <div key={i} className="flex items-center">
            <button
              type="button"
              role="tab"
              aria-selected={active}
              aria-label={`Question ${i + 1} of ${total}${answered ? " (answered)" : ""}`}
              onClick={() => onJump(i)}
              disabled={isSubmitting}
              data-testid="clarification-step"
              data-step-index={String(i)}
              data-active={active ? "true" : "false"}
              data-answered={answered ? "true" : "false"}
              className={cn(
                "h-6 w-6 rounded-full text-[11px] font-semibold flex items-center justify-center transition-colors border cursor-pointer",
                stepClassName(active, answered),
                isSubmitting ? "opacity-60 cursor-not-allowed" : "",
              )}
            >
              {answered && !active ? <IconCheck className="h-3 w-3" /> : i + 1}
            </button>
            {i < total - 1 && (
              <div
                aria-hidden="true"
                className={cn("h-px w-5 mx-0.5", isAnswered(i) ? "bg-blue-500/50" : "bg-border")}
              />
            )}
          </div>
        );
      })}
    </div>
  );
}

type OptionListProps = {
  options: ClarificationOption[];
  selectedOption: string | null;
  customCommittedText: string | null;
  isSubmitting: boolean;
  onSelectOption: (optionId: string) => void;
};

export function ClarificationOptions({
  options,
  selectedOption,
  customCommittedText,
  isSubmitting,
  onSelectOption,
}: OptionListProps) {
  return (
    <div className="space-y-1.5">
      {options.map((option, idx) => {
        const isSelected = selectedOption === option.option_id;
        const dimmed = customCommittedText !== null && !isSelected;
        return (
          <button
            key={option.option_id}
            type="button"
            onClick={() => onSelectOption(option.option_id)}
            disabled={isSubmitting}
            data-testid="clarification-option"
            data-selected={isSelected ? "true" : "false"}
            className={cn(
              "group flex items-start gap-3 w-full text-left text-sm rounded-lg px-3 py-2 transition-colors border",
              isSelected
                ? "bg-blue-500/15 border-blue-500/50 text-foreground"
                : "border-border hover:bg-muted/40 hover:border-border/80 text-foreground/90",
              isSubmitting ? "opacity-60 cursor-not-allowed" : "cursor-pointer",
              dimmed ? "opacity-60" : "",
            )}
          >
            <kbd
              aria-hidden="true"
              className="select-none font-mono text-[10px] px-1.5 py-0.5 rounded border border-border bg-muted text-muted-foreground leading-none mt-0.5"
            >
              {idx + 1}
            </kbd>
            <span className="flex-1 min-w-0">
              <span
                data-testid="clarification-option-label"
                className="block leading-5 font-medium"
              >
                {option.label}
              </span>
              {option.description && (
                <span
                  data-testid="clarification-option-description"
                  className="block text-muted-foreground/80 mt-0.5 text-xs leading-snug"
                >
                  {option.description}
                </span>
              )}
            </span>
            {isSelected && <IconCheck className="h-3.5 w-3.5 text-blue-500 mt-1 flex-shrink-0" />}
          </button>
        );
      })}
    </div>
  );
}

type CustomInputProps = {
  draft: string;
  isSubmitting: boolean;
  committedText: string | null;
  onChange: (text: string) => void;
  onSubmit: (text: string) => void;
};

export function ClarificationCustomInput({
  draft,
  isSubmitting,
  committedText,
  onChange,
  onSubmit,
}: CustomInputProps) {
  return (
    <div className="mt-2.5 flex items-center gap-2 px-3 py-2 rounded-lg border border-dashed border-border/70 bg-muted/30">
      <span className="text-muted-foreground text-xs">↳</span>
      <input
        type="text"
        placeholder={
          committedText !== null ? "Press Enter to update your answer…" : "Or type a custom answer…"
        }
        value={draft}
        onChange={(e) => onChange(e.target.value)}
        disabled={isSubmitting}
        data-testid="clarification-input"
        className="flex-1 text-sm bg-transparent placeholder:text-muted-foreground/60 focus:outline-none"
        onKeyDown={(e) => {
          if (e.key === "Enter" && !e.shiftKey && draft.trim()) {
            e.preventDefault();
            onSubmit(draft.trim());
          }
        }}
      />
      <kbd
        aria-hidden="true"
        className="select-none flex items-center gap-1 font-mono text-[10px] px-1.5 py-0.5 rounded border border-border bg-background text-muted-foreground"
      >
        <IconCornerDownLeft className="h-2.5 w-2.5" />
        Enter
      </kbd>
    </div>
  );
}

type CarouselNavProps = {
  activeIndex: number;
  total: number;
  isSubmitting: boolean;
  onPrev: () => void;
  onNext: () => void;
  onSubmit: () => void;
  canSubmit: boolean;
};

export function ClarificationCarouselNav({
  activeIndex,
  total,
  isSubmitting,
  onPrev,
  onNext,
  onSubmit,
  canSubmit,
}: CarouselNavProps) {
  const isFirst = activeIndex === 0;
  const isLast = activeIndex === total - 1;
  return (
    <div className="flex items-center justify-between gap-2 px-4 pb-3">
      <button
        type="button"
        onClick={onPrev}
        disabled={isFirst || isSubmitting}
        data-testid="clarification-prev"
        className={cn(
          "inline-flex items-center gap-1 text-xs px-2 py-1 rounded border",
          isFirst
            ? "border-transparent text-muted-foreground/40 cursor-not-allowed"
            : "border-border text-foreground/80 hover:bg-muted/50 cursor-pointer",
        )}
      >
        <IconArrowLeft className="h-3 w-3" />
        Back
      </button>
      {isLast ? (
        <button
          type="button"
          onClick={onSubmit}
          disabled={!canSubmit || isSubmitting}
          data-testid="clarification-submit"
          className={cn(
            "inline-flex items-center gap-1 text-xs px-3 py-1 rounded font-medium",
            canSubmit && !isSubmitting
              ? "bg-blue-500 text-white hover:bg-blue-500/90 cursor-pointer"
              : "bg-muted text-muted-foreground cursor-not-allowed",
          )}
        >
          {isSubmitting ? "Submitting…" : "Submit answers"}
          <IconCheck className="h-3 w-3" />
        </button>
      ) : (
        <button
          type="button"
          onClick={onNext}
          disabled={isSubmitting}
          data-testid="clarification-next"
          className="inline-flex items-center gap-1 text-xs px-2 py-1 rounded border border-border text-foreground/80 hover:bg-muted/50 cursor-pointer"
        >
          Next
          <IconArrowRight className="h-3 w-3" />
        </button>
      )}
    </div>
  );
}
