"use client";

import { IconCheck, IconCornerDownLeft, IconArrowLeft, IconArrowRight } from "@tabler/icons-react";
import { useLayoutEffect, useRef } from "react";
import { Textarea } from "@kandev/ui/textarea";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { cn } from "@/lib/utils";
import type { ClarificationOption } from "@/lib/types/http";
import { KeyboardShortcutTooltip } from "@/components/keyboard-shortcut-tooltip";
import { KEYS } from "@/lib/keyboard/constants";

// Grow the custom-answer box up to ~6 lines, then scroll internally so the
// clarification overlay stays compact.
const MAX_CUSTOM_INPUT_HEIGHT = 160;

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
  isSubmitting: boolean;
  // When the custom-text input is the active answer (user is typing or has
  // committed a draft), options visually deselect — the two are mutually
  // exclusive at commit time, so the highlight tracks whichever is active.
  customActive: boolean;
  onSelectOption: (optionId: string) => void;
};

export function ClarificationOptions({
  options,
  selectedOption,
  isSubmitting,
  customActive,
  onSelectOption,
}: OptionListProps) {
  return (
    <div className="space-y-1.5">
      {options.map((option, idx) => {
        const isSelected = !customActive && selectedOption === option.option_id;
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
  // True when the custom input is the active answer (non-empty draft or a
  // committed custom_text and no option selected). Drives the blue ring +
  // check icon so it matches the visual language of a selected option.
  active: boolean;
  onChange: (text: string) => void;
  onSubmit: (text: string) => void;
  // Called after Cmd/Ctrl+Enter so the parent can attempt a batch submit
  // (no-op when not all questions are answered yet).
  onRequestFinalSubmit?: () => void;
};

// Trailing controls for the custom-answer row. Hardware keyboards get the
// Enter/⇧↵ hints; touch devices get an inline Send button, because the overlay's
// own Submit button only renders for multi-question bundles and single-question
// custom answers would otherwise have no touch-reachable send path.
function CustomInputControls({
  isFinePointer,
  trimmed,
  isSubmitting,
  onSubmit,
}: {
  isFinePointer: boolean;
  trimmed: string;
  isSubmitting: boolean;
  onSubmit: (text: string) => void;
}) {
  if (isFinePointer) {
    return (
      <div className="mt-0.5 flex flex-shrink-0 items-center gap-1">
        <kbd
          aria-hidden="true"
          className="select-none flex items-center gap-1 font-mono text-[10px] px-1.5 py-0.5 rounded border border-border bg-background text-muted-foreground"
        >
          <IconCornerDownLeft className="h-2.5 w-2.5" />
          Enter
        </kbd>
        <span aria-hidden="true" className="select-none text-[10px] text-muted-foreground/60">
          ⇧↵ newline
        </span>
      </div>
    );
  }
  const canSend = trimmed.length > 0 && !isSubmitting;
  return (
    <button
      type="button"
      onClick={() => onSubmit(trimmed)}
      disabled={!canSend}
      data-testid="clarification-custom-submit"
      aria-label="Send answer"
      className={cn(
        "mt-0.5 flex flex-shrink-0 items-center gap-1 text-xs px-2 py-1 rounded font-medium transition-colors",
        canSend
          ? "bg-blue-500 text-white hover:bg-blue-500/90 cursor-pointer"
          : "bg-muted text-muted-foreground cursor-not-allowed",
      )}
    >
      Send
      <IconCornerDownLeft className="h-3 w-3" />
    </button>
  );
}

export function ClarificationCustomInput({
  draft,
  isSubmitting,
  committedText,
  active,
  onChange,
  onSubmit,
  onRequestFinalSubmit,
}: CustomInputProps) {
  const textareaRef = useRef<HTMLTextAreaElement>(null);
  const trimmed = draft.trim();
  // Touch keyboards have no Shift+Enter chord, so on coarse-pointer devices we
  // let Enter insert a newline and rely on the always-visible Submit button to
  // send — otherwise the multiline feature would be desktop-only.
  const { isFinePointer } = useResponsiveBreakpoint();

  // Auto-grow to fit content (WebKit lacks CSS field-sizing, so measure in JS)
  // and clamp to MAX_CUSTOM_INPUT_HEIGHT, after which the box scrolls.
  useLayoutEffect(() => {
    const el = textareaRef.current;
    if (!el) return;
    el.style.height = "auto";
    el.style.height = `${Math.min(el.scrollHeight, MAX_CUSTOM_INPUT_HEIGHT)}px`;
  }, [draft]);

  return (
    <div
      data-testid="clarification-custom-input"
      data-active={active ? "true" : "false"}
      onMouseDown={(event) => {
        if (isSubmitting) return;
        const target = event.target as HTMLElement;
        if (target.closest("textarea, button")) return;
        event.preventDefault();
        event.stopPropagation();
        textareaRef.current?.focus({ preventScroll: true });
      }}
      className={cn(
        "mt-2.5 flex min-h-11 items-start gap-2 px-3 py-2 rounded-lg border transition-colors",
        active
          ? "bg-blue-500/15 border-blue-500/50 text-foreground"
          : "border-dashed border-border/70 bg-muted/30",
        isSubmitting ? "cursor-not-allowed" : "cursor-text",
      )}
    >
      <span
        className={cn("mt-0.5 text-xs", active ? "text-blue-500" : "text-muted-foreground")}
        aria-hidden="true"
      >
        ↳
      </span>
      <Textarea
        ref={textareaRef}
        rows={1}
        placeholder={
          committedText !== null ? "Press Enter to update your answer…" : "Or type a custom answer…"
        }
        value={draft}
        onChange={(e) => onChange(e.target.value)}
        disabled={isSubmitting}
        data-testid="clarification-input"
        style={{ maxHeight: MAX_CUSTOM_INPUT_HEIGHT }}
        className="flex-1 min-h-0 resize-none overflow-y-auto border-0 bg-transparent p-0 text-sm shadow-none focus-visible:ring-0 placeholder:text-muted-foreground/60"
        onKeyDown={(e) => {
          // Shift+Enter (and Alt+Enter) fall through so the textarea inserts a
          // newline instead of submitting. isComposing ignores the Enter that
          // confirms an IME candidate (CJK keyboards).
          if (e.key !== "Enter" || e.shiftKey || e.altKey || e.nativeEvent.isComposing) return;
          if (e.metaKey || e.ctrlKey) {
            // Cmd/Ctrl+Enter only asks the parent to finalize. The draft was
            // already live-recorded via onChange, so we skip the per-question
            // commit path (which would also advance the carousel one step on
            // multi-question bundles — wasted state churn before submit).
            // e.repeat guards a held key against firing multiple finalizes.
            e.preventDefault();
            if (!e.repeat) onRequestFinalSubmit?.();
            return;
          }
          // On touch devices Enter inserts a newline (submit via the button).
          if (!isFinePointer) return;
          // Plain Enter submits on desktop. preventDefault runs unconditionally
          // (including on auto-repeat) so a held key never leaks stray newlines
          // into this — or, once the carousel advances, the next — textarea, and
          // so an empty/whitespace draft can't leak one before the trim guard.
          e.preventDefault();
          if (!e.repeat && trimmed) {
            onSubmit(trimmed);
          }
        }}
      />
      <CustomInputControls
        isFinePointer={isFinePointer}
        trimmed={trimmed}
        isSubmitting={isSubmitting}
        onSubmit={onSubmit}
      />
      {active && <IconCheck className="mt-0.5 h-3.5 w-3.5 text-blue-500 flex-shrink-0" />}
    </div>
  );
}

type CarouselNavProps = {
  activeIndex: number;
  total: number;
  isSubmitting: boolean;
  onPrev: () => void;
  onNext: () => void;
};

// Back/Next carousel nav. The final-submit affordance lives in the overlay
// header so it stays visible even when the question card scrolls past the
// fold; this nav only handles per-question navigation.
export function ClarificationCarouselNav({
  activeIndex,
  total,
  isSubmitting,
  onPrev,
  onNext,
}: CarouselNavProps) {
  const isFirst = activeIndex === 0;
  const isLast = activeIndex === total - 1;
  return (
    <div className="flex items-center justify-between gap-2 px-4 pb-3">
      <KeyboardShortcutTooltip
        shortcut={{ key: KEYS.ARROW_LEFT }}
        description="Previous question"
        enabled={!isFirst && !isSubmitting}
      >
        <span className="inline-flex">
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
        </span>
      </KeyboardShortcutTooltip>
      <KeyboardShortcutTooltip
        shortcut={{ key: KEYS.ARROW_RIGHT }}
        description="Next question"
        enabled={!isLast && !isSubmitting}
      >
        <span className="inline-flex">
          <button
            type="button"
            onClick={onNext}
            disabled={isLast || isSubmitting}
            data-testid="clarification-next"
            className={cn(
              "inline-flex items-center gap-1 text-xs px-2 py-1 rounded border",
              isLast
                ? "border-transparent text-muted-foreground/40 cursor-not-allowed"
                : "border-border text-foreground/80 hover:bg-muted/50 cursor-pointer",
            )}
          >
            Next
            <IconArrowRight className="h-3 w-3" />
          </button>
        </span>
      </KeyboardShortcutTooltip>
    </div>
  );
}
