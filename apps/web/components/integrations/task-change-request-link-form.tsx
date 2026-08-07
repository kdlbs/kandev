"use client";

import { useEffect, useId, useState } from "react";
import { Button } from "@kandev/ui/button";
import { DialogFooter } from "@kandev/ui/dialog";
import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";
import { useToast } from "@/components/toast-provider";
import { t } from "@/lib/i18n";

export type TaskChangeRequestLinkFormProps = {
  inputLabel: string;
  placeholder?: string;
  emptyError: string;
  failureMessage: string;
  successMessage: string;
  inputTestId?: string;
  errorTestId?: string;
  submitTestId?: string;
  resetKey?: unknown;
  onSubmit(reference: string): Promise<void>;
  onCancel(): void;
  onSuccess(): void;
};

/**
 * Host-owned body for task change-request linking. Provider integrations own
 * validation and transport; Kandev owns the interaction, feedback, and layout.
 */
export function TaskChangeRequestLinkForm({
  inputLabel,
  placeholder,
  emptyError,
  failureMessage,
  successMessage,
  inputTestId,
  errorTestId,
  submitTestId,
  resetKey,
  onSubmit,
  onCancel,
  onSuccess,
}: TaskChangeRequestLinkFormProps) {
  const inputId = useId();
  const { toast } = useToast();
  const [input, setInput] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    setInput("");
    setError(null);
  }, [resetKey]);

  const submit = async () => {
    const reference = input.trim();
    if (!reference) {
      setError(emptyError);
      return;
    }
    setSubmitting(true);
    setError(null);
    try {
      await onSubmit(reference);
      toast({ description: successMessage, variant: "success" });
      onSuccess();
    } catch (err) {
      setError(err instanceof Error ? err.message : failureMessage);
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <form
      className="contents"
      onSubmit={(event) => {
        event.preventDefault();
        void submit();
      }}
    >
      <div className="space-y-2">
        <Label htmlFor={inputId}>{inputLabel}</Label>
        <Input
          id={inputId}
          data-testid={inputTestId}
          value={input}
          onChange={(event) => setInput(event.target.value)}
          placeholder={placeholder}
          disabled={submitting}
          autoFocus
        />
        {error && (
          <p className="text-xs text-destructive" data-testid={errorTestId}>
            {error}
          </p>
        )}
      </div>
      <DialogFooter className="gap-2">
        <Button
          type="button"
          variant="outline"
          className="cursor-pointer"
          onClick={onCancel}
          disabled={submitting}
        >
          {t("common:cancel")}
        </Button>
        <Button
          type="submit"
          className="cursor-pointer"
          disabled={submitting}
          data-testid={submitTestId}
          data-dialog-default-action
        >
          {submitting ? t("integrations:saving") : t("common:save")}
        </Button>
      </DialogFooter>
    </form>
  );
}
