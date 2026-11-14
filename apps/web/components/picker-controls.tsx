"use client";

import { IconCheck, IconRefresh } from "@tabler/icons-react";
import { useTranslation } from "react-i18next";
import { Spinner } from "@kandev/ui/spinner";
import { controlSizingClassName } from "@kandev/ui/control-sizing";
import { cn } from "@/lib/utils";

/** Plain Enter on a text input: no modifiers, repeat, or IME composition. */
export function isPlainEnter(event: React.KeyboardEvent<HTMLInputElement>): boolean {
  return (
    event.key === "Enter" &&
    !event.defaultPrevented &&
    !event.repeat &&
    !event.altKey &&
    !event.ctrlKey &&
    !event.metaKey &&
    !event.shiftKey &&
    !event.nativeEvent.isComposing &&
    event.keyCode !== 229
  );
}

/**
 * Arrow-key focus movement between a picker's search input and its options.
 * Attach to the panel element; the input and option buttons are found by their
 * `data-picker-input` and `data-picker-option` attributes.
 */
export function movePickerFocus(event: React.KeyboardEvent<HTMLElement>): void {
  if (event.key !== "ArrowDown" && event.key !== "ArrowUp") return;
  const panel = event.currentTarget;
  const targets = [
    ...panel.querySelectorAll<HTMLElement>("[data-picker-input]"),
    ...panel.querySelectorAll<HTMLElement>("[data-picker-option]"),
  ];
  const index = targets.findIndex((target) => target === document.activeElement);
  if (index === -1) return;
  const next = event.key === "ArrowDown" ? Math.min(index + 1, targets.length - 1) : index - 1;
  event.preventDefault();
  targets[Math.max(next, 0)]!.focus();
}

/** Loading row whose translated text is the only announced status. */
export function PickerLoadingStatus({ label, testId }: { label: string; testId?: string }) {
  return (
    <div
      role="status"
      className="flex items-center gap-2 px-2 py-3 text-xs text-muted-foreground"
      data-testid={testId}
    >
      <Spinner className="size-3" role="presentation" aria-hidden="true" aria-label={undefined} />
      <span>{label}</span>
    </div>
  );
}

export type PickerRefreshControl = {
  onRefresh: () => void;
  refreshing: boolean;
  label: string;
  testId: string;
};

export type PickerSearchRowProps = {
  value: string;
  onChange: (value: string) => void;
  onKeyDown: (event: React.KeyboardEvent<HTMLInputElement>) => void;
  onPaste?: (event: React.ClipboardEvent<HTMLInputElement>) => void;
  placeholder: string;
  invalid: boolean;
  testId: string;
  legacyTestId?: string;
  refresh?: PickerRefreshControl;
};

/** Search input of a picker popover, with an optional list refresh button. */
export function PickerSearchRow({
  value,
  onChange,
  onKeyDown,
  onPaste,
  placeholder,
  invalid,
  testId,
  legacyTestId,
  refresh,
}: PickerSearchRowProps) {
  return (
    <div className="mx-2 mt-2 flex items-center gap-1">
      <input
        autoFocus
        value={value}
        onChange={(event) => onChange(event.target.value)}
        onPaste={onPaste}
        onKeyDown={onKeyDown}
        placeholder={placeholder}
        aria-label={placeholder}
        aria-invalid={invalid ? true : undefined}
        data-testid={testId}
        data-legacy-testid={legacyTestId}
        data-picker-input=""
        className={cn(
          controlSizingClassName("standard"),
          "min-w-0 flex-1 rounded-md px-2 text-xs bg-muted/30 border border-border/60",
          "outline-none focus:bg-muted focus:border-border placeholder:text-muted-foreground",
          invalid && "border-destructive focus:border-destructive",
        )}
      />
      {refresh ? <PickerRefreshButton {...refresh} /> : null}
    </div>
  );
}

function PickerRefreshButton({ onRefresh, refreshing, label, testId }: PickerRefreshControl) {
  return (
    <button
      type="button"
      onClick={onRefresh}
      disabled={refreshing}
      aria-label={label}
      title={label}
      data-testid={testId}
      className={cn(
        controlSizingClassName("icon"),
        "inline-flex shrink-0 items-center justify-center rounded-md text-muted-foreground",
        refreshing
          ? "cursor-not-allowed opacity-60"
          : "cursor-pointer hover:bg-muted hover:text-foreground",
      )}
    >
      <IconRefresh aria-hidden="true" className={cn("h-4 w-4", refreshing && "animate-spin")} />
    </button>
  );
}

export function AlreadyAddedMarker() {
  const { t } = useTranslation();
  return (
    <span
      role="img"
      aria-label={t("task:alreadyAdded")}
      data-testid="already-added-repository-marker"
      className="text-primary"
    >
      <IconCheck aria-hidden="true" className="h-4 w-4" />
    </span>
  );
}
