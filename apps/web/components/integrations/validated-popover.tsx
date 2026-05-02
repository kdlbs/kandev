"use client";

import { useCallback, useRef, useState, type ButtonHTMLAttributes, type ReactNode } from "react";
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
  // testIdPrefix scopes the data-testid attributes on trigger / input / submit
  // / error so callers can target the right popover when more than one is
  // mounted on the same page (e.g. a Jira import bar next to a Linear one).
  testIdPrefix?: string;
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

type TriggerButtonProps = Pick<
  ValidatedPopoverProps<unknown>,
  | "triggerStyle"
  | "triggerIcon"
  | "triggerLabel"
  | "triggerAriaLabel"
  | "triggerDisabled"
  | "testIdPrefix"
> &
  ButtonHTMLAttributes<HTMLButtonElement>;

// TriggerButton forwards every prop the parent injects (notably radix Slot's
// onClick / onPointerDown / aria-expanded / data-state from PopoverTrigger and
// TooltipTrigger asChild) onto the inner Button. Forgetting `...rest` here
// silently drops radix's open-toggle handler, which makes the popover
// unclickable while leaving the tooltip working — easy to miss in dev.
function TriggerButton({
  triggerStyle,
  triggerIcon,
  triggerLabel,
  triggerAriaLabel,
  triggerDisabled,
  testIdPrefix,
  ...rest
}: TriggerButtonProps) {
  const triggerTestId = testIdPrefix ? `${testIdPrefix}-trigger` : undefined;
  if (triggerStyle === "outline-with-label") {
    return (
      <Button
        type="button"
        size="sm"
        variant="outline"
        disabled={triggerDisabled}
        className="cursor-pointer px-2 gap-1"
        data-testid={triggerTestId}
        {...rest}
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
      data-testid={triggerTestId}
      {...rest}
    >
      {triggerIcon}
    </Button>
  );
}

type PopoverBodyProps = {
  headline: string;
  placeholder: string;
  value: string;
  onChange: (next: string) => void;
  onSubmit: () => void | Promise<void>;
  loading: boolean;
  error: string | null;
  submitLabel: string;
  submittingLabel: string;
  testIdPrefix?: string;
};

function PopoverBody({
  headline,
  placeholder,
  value,
  onChange,
  onSubmit,
  loading,
  error,
  submitLabel,
  submittingLabel,
  testIdPrefix,
}: PopoverBodyProps) {
  return (
    <div className="space-y-2">
      <div className="text-xs font-medium">{headline}</div>
      <Input
        autoFocus
        value={value}
        onChange={(e) => onChange(e.target.value)}
        placeholder={placeholder}
        className="h-8 text-xs"
        data-testid={testIdPrefix ? `${testIdPrefix}-input` : undefined}
        onKeyDown={(e) => {
          if (e.key === "Enter" && !e.shiftKey) {
            e.preventDefault();
            void onSubmit();
          }
        }}
      />
      {error && (
        <p
          className="text-[11px] text-destructive"
          role="alert"
          data-testid={testIdPrefix ? `${testIdPrefix}-error` : undefined}
        >
          {error}
        </p>
      )}
      <div className="flex justify-end">
        <Button
          type="button"
          size="sm"
          onClick={() => void onSubmit()}
          disabled={loading || !value.trim()}
          className="h-7 cursor-pointer"
          data-testid={testIdPrefix ? `${testIdPrefix}-submit` : undefined}
        >
          {loading ? submittingLabel : submitLabel}
        </Button>
      </div>
    </div>
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
    testIdPrefix,
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
            <TriggerButton
              triggerStyle={props.triggerStyle}
              triggerIcon={props.triggerIcon}
              triggerLabel={props.triggerLabel}
              triggerAriaLabel={props.triggerAriaLabel}
              triggerDisabled={props.triggerDisabled}
              testIdPrefix={props.testIdPrefix}
            />
          </PopoverTrigger>
        </TooltipTrigger>
        <TooltipContent>{tooltip}</TooltipContent>
      </Tooltip>
      <PopoverContent align={align ?? "end"} className="w-80 p-3">
        <PopoverBody
          headline={headline}
          placeholder={placeholder}
          value={value}
          onChange={setValue}
          onSubmit={submit}
          loading={loading}
          error={error}
          submitLabel={submitLabel}
          submittingLabel={submittingLabel}
          testIdPrefix={testIdPrefix}
        />
      </PopoverContent>
    </Popover>
  );
}
