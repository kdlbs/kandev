"use client";

import { useState, type FormEvent } from "react";
import { useTranslation } from "react-i18next";
import { Button } from "@kandev/ui/button";
import { Checkbox } from "@kandev/ui/checkbox";
import { FieldError } from "@kandev/ui/field";
import { Input } from "@kandev/ui/input";
import { Label } from "@kandev/ui/label";
import { RadioGroup, RadioGroupItem } from "@kandev/ui/radio-group";
import { Spinner } from "@kandev/ui/spinner";
import type {
  AnswerDraft,
  AnswerDraftEntry,
  AnswerSnapshotPermission,
  AnswerSnapshotQuestion,
  AnswerSpec,
  ApproveProposalEdits,
} from "@/lib/api/domains/coordinator-api";
import type { EditFormServerError } from "./edit-form";

/** The permission options a draft may choose: one-shot allow or reject only. */
const DRAFT_PERMISSION_KINDS: ReadonlySet<string> = new Set(["allow_once", "reject_once"]);

type AnswerDraftEditorProps = {
  spec: AnswerSpec;
  draft: AnswerDraft;
  busy: boolean;
  serverError: EditFormServerError | null;
  onApprove: (edits: ApproveProposalEdits) => void;
  onCancel: () => void;
};

function entryFor(draft: AnswerDraft, questionId: string): AnswerDraftEntry {
  return (
    draft.answers?.find((entry) => entry.question_id === questionId) ?? {
      question_id: questionId,
    }
  );
}

function QuestionEditor({
  question,
  entry,
  disabled,
  onChange,
}: {
  question: AnswerSnapshotQuestion;
  entry: AnswerDraftEntry;
  disabled: boolean;
  onChange: (entry: AnswerDraftEntry) => void;
}) {
  const { t } = useTranslation();
  const chosen = new Set(entry.option_ids ?? []);
  function toggle(optionId: string, on: boolean) {
    const next = question.options
      .map((o) => o.id)
      .filter((id) => (id === optionId ? on : chosen.has(id)));
    onChange({ ...entry, option_ids: next });
  }
  return (
    <fieldset className="space-y-2" data-testid={`answer-edit-question-${question.id}`}>
      <legend className="text-sm font-medium">{question.text}</legend>
      {question.options.map((option) => (
        <Label
          key={option.id}
          className="flex min-h-11 cursor-pointer items-center gap-2 sm:min-h-0"
        >
          <Checkbox
            checked={chosen.has(option.id)}
            disabled={disabled}
            onCheckedChange={(on) => toggle(option.id, on === true)}
          />
          {option.label}
        </Label>
      ))}
      {question.allow_custom_text !== false && (
        <Input
          aria-label={t("coordinator:answerCustomText")}
          placeholder={t("coordinator:answerCustomText")}
          value={entry.custom_text ?? ""}
          disabled={disabled}
          onChange={(e) => onChange({ ...entry, custom_text: e.target.value })}
        />
      )}
    </fieldset>
  );
}

function PermissionEditor({
  permission,
  optionId,
  disabled,
  onChange,
}: {
  permission: AnswerSnapshotPermission;
  optionId: string;
  disabled: boolean;
  onChange: (optionId: string) => void;
}) {
  const options = permission.options.filter((o) => DRAFT_PERMISSION_KINDS.has(o.kind));
  return (
    <RadioGroup value={optionId} onValueChange={onChange} disabled={disabled}>
      <p className="text-sm font-medium">{permission.title}</p>
      {options.map((option) => (
        <Label
          key={option.id}
          className="flex min-h-11 cursor-pointer items-center gap-2 sm:min-h-0"
        >
          <RadioGroupItem value={option.id} />
          {option.name}
        </Label>
      ))}
    </RadioGroup>
  );
}

function isComplete(spec: AnswerSpec, draft: AnswerDraft): boolean {
  if (spec.target === "permission") return (draft.option_id ?? "") !== "";
  return (spec.snapshot.questions ?? []).every((q) => {
    const entry = entryFor(draft, q.id);
    return (entry.option_ids?.length ?? 0) > 0 || (entry.custom_text ?? "").trim() !== "";
  });
}

function normalized(spec: AnswerSpec, draft: AnswerDraft): AnswerDraft {
  if (spec.target === "permission") return { option_id: draft.option_id };
  return {
    answers: (spec.snapshot.questions ?? []).map((q) => {
      const entry = entryFor(draft, q.id);
      const text = (entry.custom_text ?? "").trim();
      return {
        question_id: q.id,
        ...(entry.option_ids?.length ? { option_ids: entry.option_ids } : {}),
        ...(text ? { custom_text: text } : {}),
      };
    }),
  };
}

/**
 * The Edit view of an answer card: draft-only controls over the stored
 * snapshot. Approving sends the whole draft as the top-level `draft` edit.
 */
export function AnswerDraftEditor({
  spec,
  draft: initial,
  busy,
  serverError,
  onApprove,
  onCancel,
}: AnswerDraftEditorProps) {
  const { t } = useTranslation();
  const [draft, setDraft] = useState<AnswerDraft>(initial);
  const valid = isComplete(spec, draft);

  function setEntry(entry: AnswerDraftEntry) {
    const others = (draft.answers ?? []).filter((e) => e.question_id !== entry.question_id);
    setDraft({ ...draft, answers: [...others, entry] });
  }
  function handleSubmit(e: FormEvent) {
    e.preventDefault();
    if (busy || !valid) return;
    onApprove({ draft: normalized(spec, draft) });
  }

  return (
    <form className="space-y-3" onSubmit={handleSubmit} data-testid="answer-draft-editor">
      {spec.target === "permission" && spec.snapshot.permission ? (
        <PermissionEditor
          permission={spec.snapshot.permission}
          optionId={draft.option_id ?? ""}
          disabled={busy}
          onChange={(option_id) => setDraft({ option_id })}
        />
      ) : (
        (spec.snapshot.questions ?? []).map((q) => (
          <QuestionEditor
            key={q.id}
            question={q}
            entry={entryFor(draft, q.id)}
            disabled={busy}
            onChange={setEntry}
          />
        ))
      )}
      {serverError && <FieldError>{serverError.message}</FieldError>}
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
