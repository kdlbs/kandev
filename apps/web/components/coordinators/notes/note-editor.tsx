"use client";

import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { Label } from "@kandev/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { Textarea } from "@kandev/ui/textarea";
import {
  NOTE_TYPES,
  type NoteDraft,
  type NoteFailure,
  type NoteType,
} from "@/lib/api/domains/coordinator-notes-api";
import {
  NOTE_TEXT_MAX,
  citationComplete,
  noteFailureMessage,
  normalizedCitation,
  noteTextLength,
  noteTypeLabel,
} from "@/lib/coordinator/notes";
import { CitationEditor } from "./citation-editor";

type NoteEditorProps = {
  initial: NoteDraft;
  /** Resolves to the rejection, or null once the write was accepted. */
  onSubmit: (draft: NoteDraft) => Promise<NoteFailure | null>;
  onCancel: () => void;
  /** Rereads the stored note; offered only after a version conflict. */
  onReload?: () => Promise<void>;
  /** Starts a new note from the draft; offered only when the stored note can no longer change. */
  onCreateFromDraft?: (draft: NoteDraft) => void;
};

function ConflictActions(props: {
  failure: NoteFailure;
  busy: boolean;
  onReload?: () => void;
  onCreate?: () => void;
}) {
  const { t } = useTranslation();
  const { failure, busy, onReload, onCreate } = props;
  if (failure.code !== "conflict") return null;
  const frozen = failure.reason === "expired" || failure.reason === "deleted";
  return (
    <div className="flex flex-wrap gap-2" data-testid="note-conflict-actions">
      {!frozen && onReload && (
        <Button
          type="button"
          variant="outline"
          disabled={busy}
          className="min-h-11 cursor-pointer sm:min-h-8"
          data-testid="note-conflict-reload"
          onClick={onReload}
        >
          {t("coordinator:noteConflictReload")}
        </Button>
      )}
      {frozen && onCreate && (
        <Button
          type="button"
          className="min-h-11 cursor-pointer sm:min-h-8"
          data-testid="note-conflict-create"
          onClick={onCreate}
        >
          {t("coordinator:noteConflictCreateFromDraft")}
        </Button>
      )}
    </div>
  );
}

function conflictText(t: ReturnType<typeof useTranslation>["t"], failure: NoteFailure): string {
  if (failure.reason === "expired") return t("coordinator:noteConflictExpired");
  if (failure.reason === "deleted") return t("coordinator:noteConflictDeleted");
  if (failure.reason === "version") return t("coordinator:noteConflictVersion");
  return noteFailureMessage(t, failure);
}

function draftValid(draft: NoteDraft, length: number): boolean {
  if (length === 0 || length > NOTE_TEXT_MAX) return false;
  return draft.citations.length > 0 && draft.citations.every(citationComplete);
}

function NoteFields(props: {
  draft: NoteDraft;
  length: number;
  disabled: boolean;
  onChange: (next: NoteDraft) => void;
}) {
  const { t } = useTranslation();
  const { draft, length, disabled, onChange } = props;
  return (
    <>
      <div className="space-y-1">
        <Label htmlFor="note-type">{t("coordinator:noteTypeLabel")}</Label>
        <Select
          value={draft.type}
          disabled={disabled}
          onValueChange={(type) => onChange({ ...draft, type: type as NoteType })}
        >
          <SelectTrigger id="note-type" className="min-h-11 w-full cursor-pointer sm:min-h-8">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {NOTE_TYPES.map((type) => (
              <SelectItem key={type} value={type}>
                {noteTypeLabel(t, type)}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>
      <div className="space-y-1">
        <Label htmlFor="note-text">{t("coordinator:noteTextLabel")}</Label>
        <Textarea
          id="note-text"
          rows={4}
          value={draft.text}
          disabled={disabled}
          data-testid="note-text"
          onChange={(event) => onChange({ ...draft, text: event.target.value })}
        />
        <p className="text-xs text-muted-foreground" data-testid="note-text-count">
          {t("coordinator:noteTextCount", { length, max: NOTE_TEXT_MAX })}
        </p>
      </div>
    </>
  );
}

/** A note draft form. A failed write keeps the draft; nothing is reloaded unless the user asks. */
export function NoteEditor({
  initial,
  onSubmit,
  onCancel,
  onReload,
  onCreateFromDraft,
}: NoteEditorProps) {
  const { t } = useTranslation();
  const [draft, setDraft] = useState<NoteDraft>(initial);
  const [busy, setBusy] = useState(false);
  const [failure, setFailure] = useState<NoteFailure | null>(null);
  const frozen = failure?.code === "conflict" && failure.reason !== "version";
  const length = noteTextLength(draft.text);
  const valid = draftValid(draft, length);

  const submit = async () => {
    setBusy(true);
    const next = await onSubmit({ ...draft, citations: draft.citations.map(normalizedCitation) });
    setBusy(false);
    setFailure(next);
  };
  const reload = async () => {
    setBusy(true);
    await onReload?.();
    setBusy(false);
    setFailure(null);
  };

  return (
    <form
      className="space-y-4"
      data-testid="note-editor"
      onSubmit={(event) => {
        event.preventDefault();
        if (valid && !busy && !frozen) void submit();
      }}
    >
      <NoteFields draft={draft} length={length} disabled={busy || frozen} onChange={setDraft} />
      <CitationEditor
        value={draft.citations}
        disabled={busy || frozen}
        failedIndex={failure?.code === "citation_unresolved" ? failure.index : undefined}
        onChange={(citations) => setDraft({ ...draft, citations })}
      />
      {failure && (
        <p role="alert" className="text-sm text-destructive" data-testid="note-editor-error">
          {failure.code === "conflict" ? conflictText(t, failure) : noteFailureMessage(t, failure)}
        </p>
      )}
      {failure && (
        <ConflictActions
          failure={failure}
          busy={busy}
          onReload={onReload ? () => void reload() : undefined}
          onCreate={onCreateFromDraft ? () => onCreateFromDraft(draft) : undefined}
        />
      )}
      <div className="flex flex-wrap gap-2">
        <Button
          type="submit"
          disabled={!valid || busy || frozen}
          className="min-h-11 cursor-pointer sm:min-h-8"
          data-testid="note-save"
        >
          {t("coordinator:noteSave")}
        </Button>
        <Button
          type="button"
          variant="outline"
          disabled={busy}
          className="min-h-11 cursor-pointer sm:min-h-8"
          data-testid="note-cancel"
          onClick={onCancel}
        >
          {t("coordinator:noteCancel")}
        </Button>
      </div>
    </form>
  );
}
