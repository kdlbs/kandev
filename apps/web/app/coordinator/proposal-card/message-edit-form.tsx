"use client";

import { useEffect, useId, useRef, useState, type FormEvent } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { Field, FieldContent, FieldError, FieldLabel } from "@kandev/ui/field";
import { Spinner } from "@kandev/ui/spinner";
import { Textarea } from "@kandev/ui/textarea";
import type { ApproveProposalEdits } from "@/lib/api/domains/coordinator-api";
import type { EditFormServerError } from "./edit-form";
import { LONG_TEXT_FIELD_CLASS } from "@/components/coordinators/long-text-field";

export const MAX_MESSAGE_CODE_POINTS = 4000;

type MessageEditFormProps = {
  text: string;
  busy: boolean;
  serverError: EditFormServerError | null;
  onApprove: (edits: ApproveProposalEdits) => void;
  onCancel: () => void;
};

/** The Edit form of a message card: the text only, trimmed, 1 to 4000 code points. */
export function MessageEditForm({
  text,
  busy,
  serverError,
  onApprove,
  onCancel,
}: MessageEditFormProps) {
  const { t } = useTranslation();
  const fieldId = useId();
  const [value, setValue] = useState(text);
  const areaRef = useRef<HTMLTextAreaElement>(null);
  const trimmed = value.trim();
  const used = Array.from(trimmed).length;
  const valid = used > 0 && used <= MAX_MESSAGE_CODE_POINTS;

  useEffect(() => {
    areaRef.current?.focus();
  }, []);

  function handleSubmit(e: FormEvent) {
    e.preventDefault();
    if (busy || !valid) return;
    onApprove({ text: trimmed });
  }

  return (
    <form className="space-y-3" onSubmit={handleSubmit}>
      <Field>
        <FieldContent>
          <FieldLabel htmlFor={fieldId}>{t("coordinator:editMessageLabel")}</FieldLabel>
          <Textarea
            id={fieldId}
            className={LONG_TEXT_FIELD_CLASS}
            ref={areaRef}
            value={value}
            disabled={busy}
            onChange={(e) => setValue(e.target.value)}
          />
          <p className="text-muted-foreground text-xs/relaxed">
            {t("coordinator:messageCounter", { used, max: MAX_MESSAGE_CODE_POINTS })}
          </p>
          {serverError && <FieldError>{serverError.message}</FieldError>}
        </FieldContent>
      </Field>
      <div className="flex flex-wrap gap-2">
        <Button type="submit" size="sm" disabled={busy || !valid} className="min-h-11 sm:min-h-0">
          {busy && <Spinner aria-hidden className="mr-1.5" />}
          {t("coordinator:approveWithEdits")}
        </Button>
        <Button
          type="button"
          size="sm"
          variant="outline"
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
