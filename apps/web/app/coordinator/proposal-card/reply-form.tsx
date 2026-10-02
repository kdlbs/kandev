"use client";

import { useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { Field, FieldContent, FieldError, FieldLabel } from "@kandev/ui/field";
import { Spinner } from "@kandev/ui/spinner";
import { Textarea } from "@kandev/ui/textarea";
import { REPLY_TEXT_MAX, replyTextLength } from "@/hooks/domains/coordinator/use-proposal-reply";
import { LONG_TEXT_FIELD_CLASS } from "@/components/coordinators/long-text-field";

export type ReplyFormServerError = { message: string; field: string | null };

export type ReplyFormProps = {
  busy: boolean;
  serverError: ReplyFormServerError | null;
  onSend: (text: string) => void;
  onCancel: () => void;
};

export function ReplyForm({ busy, serverError, onSend, onCancel }: ReplyFormProps) {
  const { t } = useTranslation();
  const [text, setText] = useState("");
  const textRef = useRef<HTMLTextAreaElement>(null);
  const alertRef = useRef<HTMLDivElement>(null);
  const trimmed = text.trim();
  const length = replyTextLength(trimmed);
  const valid = length >= 1 && length <= REPLY_TEXT_MAX;

  useEffect(() => {
    textRef.current?.focus();
  }, []);

  useEffect(() => {
    if (!serverError) return;
    (serverError.field === "text" ? textRef : alertRef).current?.focus();
  }, [serverError]);

  return (
    <form
      className="space-y-3"
      onSubmit={(e) => {
        e.preventDefault();
        if (!busy && valid) onSend(trimmed);
      }}
    >
      <Field>
        <FieldContent>
          <FieldLabel htmlFor="proposal-reply-text">{t("coordinator:replyLabel")}</FieldLabel>
          <Textarea
            id="proposal-reply-text"
            className={LONG_TEXT_FIELD_CLASS}
            ref={textRef}
            value={text}
            disabled={busy}
            onChange={(e) => setText(e.target.value)}
            placeholder={t("coordinator:replyPlaceholder")}
          />
          <p
            data-testid="proposal-reply-counter"
            className={
              length > REPLY_TEXT_MAX
                ? "text-destructive text-xs/relaxed"
                : "text-muted-foreground text-xs/relaxed"
            }
          >
            {t("coordinator:replyCounter", { length, max: REPLY_TEXT_MAX })}
          </p>
          {serverError?.field === "text" && <FieldError>{serverError.message}</FieldError>}
        </FieldContent>
      </Field>
      {serverError && serverError.field !== "text" && (
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
        <Button type="submit" size="sm" disabled={busy || !valid} className="min-h-11 sm:min-h-0">
          {busy && <Spinner aria-hidden className="mr-1.5" />}
          {t("coordinator:replySend")}
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
