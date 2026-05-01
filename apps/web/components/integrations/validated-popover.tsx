"use client";

import { useCallback, useRef, useState, type ReactNode } from "react";
import { Button } from "@kandev/ui/button";
import { Input } from "@kandev/ui/input";
import { Popover, PopoverContent, PopoverTrigger } from "@kandev/ui/popover";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";

// ValidatedPopover is the shared trigger+popover+input+submit skeleton both
// integrations need for "paste a key/URL → fetch the entity → do something
// with it" flows (link a task, import a ticket/issue). It owns:
//   - open/close + state reset on close
//   - the keyed Input with autofocus + Enter-to-submit
//   - validation against an integration-supplied regex
//   - loading + error display
//   - the disabled-while-empty / disabled-while-loading submit Button
//
// Each integration provides {icon, label, tooltip, placeholder, regex,
// fetch, onSuccess, validationHint, headline, submitLabel, submittingLabel}.
//
// Designing this as one shared skeleton both fixes the existing bug that
// JiraLinkButton was missing the clear-error-on-close behaviour LinearLinkButton
// had, and prevents future drift between the four near-identical popovers.

export type ValidatedPopoverTriggerStyle = "outline-with-label" | "ghost-icon";

export type ValidatedPopoverProps<T> = {
  // Trigger button content + tooltip.
  triggerStyle: ValidatedPopoverTriggerStyle;
  triggerIcon: ReactNode;
  triggerLabel?: string; // Required for "outline-with-label", ignored for "ghost-icon".
  triggerAriaLabel?: string; // Used for "ghost-icon" since there's no visible label.
  triggerDisabled?: boolean;
  tooltip: string;
  // PopoverContent layout.
  align?: "start" | "end";
  headline: string;
  placeholder: string;
  // Validation: extract a key from the user's input. Returning null shows the
  // hint as an error. The hint typically reads "Paste a Jira ticket URL or
  // key (PROJ-123)" or similar — integration-specific copy.
  extractKey: (rawValue: string) => string | null;
  validationHint: string;
  // Async work to run after the user submits a valid key. The result is
  // handed to onSuccess; throwing surfaces .message as the error string.
  fetch: (key: string) => Promise<T>;
  onSuccess: (key: string, result: T) => void;
  // Submit button labels.
  submitLabel: string; // e.g. "Link", "Import"
  submittingLabel: string; // e.g. "Linking...", "Loading..."
};

function TriggerButton({
  triggerStyle,
  triggerIcon,
  triggerLabel,
  triggerAriaLabel,
  triggerDisabled,
}: Pick<
  ValidatedPopoverProps<unknown>,
  "triggerStyle" | "triggerIcon" | "triggerLabel" | "triggerAriaLabel" | "triggerDisabled"
>) {
  if (triggerStyle === "outline-with-label") {
    return (
      <Button
        type="button"
        size="sm"
        variant="outline"
        disabled={triggerDisabled}
        className="cursor-pointer px-2 gap-1"
      >
        {triggerIcon}
        <span className="text-xs font-medium">{triggerLabel}</span>
      </Button>
    );
  }
  return (
    <Button
      type="button"
      variant="ghost"
      size="icon"
      disabled={triggerDisabled}
      aria-label={triggerAriaLabel}
      className="h-7 w-7 cursor-pointer hover:bg-muted/40 text-slate-400"
    >
      {triggerIcon}
    </Button>
  );
}

export function ValidatedPopover<T>(props: ValidatedPopoverProps<T>) {
  const {
    tooltip,
    align,
    headline,
    placeholder,
    extractKey,
    validationHint,
    fetch,
    onSuccess,
    submitLabel,
    submittingLabel,
  } = props;
  const [open, setOpen] = useState(false);
  const [value, setValue] = useState("");
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  // Submission token: bumped on every submit and on close, so an in-flight
  // request whose promise resolves after the popover was closed (or
  // re-opened) cannot leak loading/error state into the next session.
  const submissionRef = useRef(0);

  const submit = useCallback(async () => {
    const key = extractKey(value.trim());
    if (!key) {
      setError(validationHint);
      return;
    }
    const submission = ++submissionRef.current;
    setLoading(true);
    setError(null);
    try {
      const result = await fetch(key);
      if (submission !== submissionRef.current) return;
      onSuccess(key, result);
      setOpen(false);
      setValue("");
    } catch (err) {
      if (submission !== submissionRef.current) return;
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      if (submission === submissionRef.current) setLoading(false);
    }
  }, [value, extractKey, validationHint, fetch, onSuccess]);

  // Closing invalidates any in-flight submit (so a late rejection can't
  // re-populate `error` after the user dismissed the popover) and clears
  // local submit state so the next open starts clean.
  const handleOpenChange = (next: boolean) => {
    setOpen(next);
    if (!next) {
      submissionRef.current += 1;
      setLoading(false);
      setError(null);
    }
  };

  return (
    <Popover open={open} onOpenChange={handleOpenChange}>
      <Tooltip>
        <TooltipTrigger asChild>
          <PopoverTrigger asChild>
            <TriggerButton {...props} />
          </PopoverTrigger>
        </TooltipTrigger>
        <TooltipContent>{tooltip}</TooltipContent>
      </Tooltip>
      <PopoverContent align={align ?? "end"} className="w-80 p-3">
        <div className="space-y-2">
          <div className="text-xs font-medium">{headline}</div>
          <Input
            autoFocus
            value={value}
            onChange={(e) => setValue(e.target.value)}
            placeholder={placeholder}
            className="h-8 text-xs"
            onKeyDown={(e) => {
              if (e.key === "Enter" && !e.shiftKey) {
                e.preventDefault();
                void submit();
              }
            }}
          />
          {error && (
            <p className="text-[11px] text-destructive" role="alert">
              {error}
            </p>
          )}
          <div className="flex justify-end">
            <Button
              type="button"
              size="sm"
              onClick={() => void submit()}
              disabled={loading || !value.trim()}
              className="h-7 cursor-pointer"
            >
              {loading ? submittingLabel : submitLabel}
            </Button>
          </div>
        </div>
      </PopoverContent>
    </Popover>
  );
}
