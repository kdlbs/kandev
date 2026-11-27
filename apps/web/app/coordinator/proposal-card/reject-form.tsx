"use client";

import { useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { Field, FieldContent, FieldError, FieldLabel } from "@kandev/ui/field";
import { Spinner } from "@kandev/ui/spinner";
import { Textarea } from "@kandev/ui/textarea";

export type RejectFormServerError = { message: string; field: string | null };

export type RejectFormProps = {
  busy: boolean;
  serverError: RejectFormServerError | null;
  onConfirm: (reason: string | undefined) => void;
  onCancel: () => void;
};

/**
 * The Reject form: an optional reason, Confirm reject and Cancel
 * (docs/specs/coordinator/requirements/proposals.md AC-COORDINATOR-
 * PROPOSALS-005.5).
 */
export function RejectForm({ busy, serverError, onConfirm, onCancel }: RejectFormProps) {
  const { t } = useTranslation();
  const [reason, setReason] = useState("");
  const reasonRef = useRef<HTMLTextAreaElement>(null);
  const alertRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    reasonRef.current?.focus();
  }, []);

  useEffect(() => {
    if (serverError && (!serverError.field || serverError.field !== "reason")) {
      alertRef.current?.focus();
    } else if (serverError?.field === "reason") {
      reasonRef.current?.focus();
    }
  }, [serverError]);

  return (
    <form
      className="space-y-3"
      onSubmit={(e) => {
        e.preventDefault();
        if (!busy) onConfirm(reason.trim() ? reason.trim() : undefined);
      }}
    >
      <Field>
        <FieldContent>
          <FieldLabel htmlFor="proposal-reject-reason">
            {t("coordinator:rejectReasonLabel")}
          </FieldLabel>
          <Textarea
            id="proposal-reject-reason"
            ref={reasonRef}
            value={reason}
            disabled={busy}
            onChange={(e) => setReason(e.target.value)}
            placeholder={t("coordinator:rejectReasonPlaceholder")}
          />
          {serverError?.field === "reason" && <FieldError>{serverError.message}</FieldError>}
        </FieldContent>
      </Field>
      {serverError && (!serverError.field || serverError.field !== "reason") && (
        <div
          ref={alertRef}
          role="alert"
          tabIndex={-1}
          className="text-destructive text-xs/relaxed font-normal"
        >
          {serverError.message}
        </div>
      )}
      <div className="flex flex-wrap gap-2">
        <Button type="submit" size="sm" disabled={busy} className="min-h-11 sm:min-h-0">
          {busy && <Spinner aria-hidden className="mr-1.5" />}
          {t("coordinator:confirmReject")}
        </Button>
        <Button
          type="button"
          variant="outline"
          size="sm"
          disabled={busy}
          onClick={onCancel}
          className="min-h-11 sm:min-h-0"
        >
          {t("coordinator:cancel")}
        </Button>
      </div>
    </form>
  );
}
